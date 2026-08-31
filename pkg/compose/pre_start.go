/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// preStartHookType is stored in the HookLabel on every pre_start hook container
// so orphan containers from a previous failed run can be identified and removed
// by a project+service+hook label filter.
const preStartHookType = "pre_start"

// lowestNumberedContainer returns the container with the lowest
// com.docker.compose.container-number label, so pre_start always targets the
// same replica regardless of the order the daemon returned them in.
// Panics on an empty slice; callers must guard.
func lowestNumberedContainer(containers Containers) container.Summary {
	pick := containers[0]
	pickNum, _ := strconv.Atoi(pick.Labels[api.ContainerNumberLabel])
	for _, ctr := range containers[1:] {
		num, _ := strconv.Atoi(ctr.Labels[api.ContainerNumberLabel])
		if num < pickNum {
			pick, pickNum = ctr, num
		}
	}
	return pick
}

// runPreStart executes the service's pre_start hooks sequentially, in declared
// order. Each hook runs as an ephemeral container that shares the service
// container's volumes via VolumesFrom and is attached to the same networks.
// A non-zero exit gates service start.
//
// With per_replica: false (the only currently supported mode), the hook sees
// the volumes of the first non-running replica only — anonymous volumes and
// tmpfs mounts are per-replica and not shared. Use named volumes or bind
// mounts for data the hook produces.
func (s *composeService) runPreStart(ctx context.Context, project *types.Project, service types.ServiceConfig, ctr container.Summary, listener api.ContainerEventListener) error {
	// Validate every hook up front so an unsupported entry never triggers any I/O.
	for i, hook := range service.PreStart {
		if hook.PerReplica {
			return fmt.Errorf("service %q pre_start[%d]: per_replica is not yet supported; remove per_replica or set it to false", service.Name, i)
		}
	}
	// Remove any hook containers left behind by a previous failed run so they do
	// not accumulate. Only one orphan can exist per service (failure gates the
	// remaining hooks), but we clean the whole set in case the service definition
	// changed between runs. Removal failures are non-fatal: they are logged so
	// the operator can identify the stale container.
	if err := s.removeOrphanPreStartContainers(ctx, project.Name, service.Name); err != nil {
		logrus.Warnf("service %q: failed to remove stale pre_start hook containers: %v", service.Name, err)
	}
	for i, hook := range service.PreStart {
		if err := s.runPreStartHook(ctx, project, service, ctr, i, hook, listener); err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) runPreStartHook(
	ctx context.Context, project *types.Project, service types.ServiceConfig,
	ctr container.Summary, index int, hook types.PreStartHook, listener api.ContainerEventListener,
) error {
	created, err := s.createPreStartContainer(ctx, project, service, ctr, hook)
	if err != nil {
		return err
	}

	// Subscribe to wait before start to avoid missing the exit event for short-lived hooks.
	// WaitConditionNotRunning would match immediately because the container is still in
	// "created" state, so use WaitConditionNextExit to block until the run actually finishes.
	waitRes := s.apiClient().ContainerWait(ctx, created.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNextExit,
	})

	// Open the log stream before ContainerStart so a fast-exiting hook cannot
	// race us to a 404. The dedicated logCtx lets us force the follow stream
	// closed once the hook has exited, so a daemon that keeps the connection
	// open cannot deadlock `<-logsDone`.
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()
	logsDone, getTail := s.streamPreStartLogs(logCtx, created.ID, service, index, listener)

	if _, err := s.apiClient().ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		// AutoRemove is false, so we must remove the never-started container
		// explicitly. A failed removal is logged so the orphan is visible.
		if _, removeErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); removeErr != nil {
			logrus.Warnf("service %q pre_start[%d]: failed to remove orphan hook container %s: %v", service.Name, index, created.ID, removeErr)
		}
		// Drain waitRes so the client's wait goroutine exits without having to
		// wait for the parent context to be canceled.
		select {
		case <-waitRes.Error:
		case <-waitRes.Result:
		case <-ctx.Done():
		}
		cancelLogs()
		<-logsDone
		return err
	}

	waitErr := waitPreStart(ctx, service.Name, index, waitRes)
	cancelLogs()
	<-logsDone
	if waitErr != nil {
		// Ctrl-C is a user cancellation, not a hook failure: remove the container
		// and return the raw context error without decorating it with the tail or
		// retaining the container for post-mortem inspection.
		if ctx.Err() != nil {
			if _, removeErr := s.apiClient().ContainerRemove(context.Background(), created.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); removeErr != nil {
				logrus.Warnf("service %q pre_start[%d]: failed to remove hook container %s after cancellation: %v", service.Name, index, created.ID, removeErr)
			}
			return waitErr
		}
		// Genuine hook failure: retain the container so the operator can run
		// `docker logs <id>` and `docker inspect <id>` to diagnose the failure.
		// Include the short container ID in the error to make it actionable.
		shortID := created.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		if tail := getTail(); tail != "" {
			return fmt.Errorf("%w: %s (hook container %s retained for inspection)", waitErr, tail, shortID)
		}
		return fmt.Errorf("%w (hook container %s retained for inspection)", waitErr, shortID)
	}
	// Success: remove the hook container, mirroring the old AutoRemove behaviour
	// (including its anonymous volumes). A removal failure is logged but does not
	// gate service start — the hook already succeeded.
	if _, removeErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{RemoveVolumes: true}); removeErr != nil {
		logrus.Warnf("service %q pre_start[%d]: failed to remove hook container %s: %v", service.Name, index, created.ID, removeErr)
	}
	return nil
}

func (s *composeService) createPreStartContainer(
	ctx context.Context, project *types.Project, service types.ServiceConfig,
	ctr container.Summary, hook types.PreStartHook,
) (client.ContainerCreateResult, error) {
	// A pre_start hook is a full container specification (compose-spec#656),
	// already resolved by compose-go at load time: every service attribute
	// the hook doesn't override — resources, capabilities, dns, sysctls,
	// image, ... — is inherited in the model itself, and the spec runs
	// through the standard create path exactly as a service container would.
	// Only volumes inherit here, at runtime, through volumes_from below.
	spec := hook.ContainerSpec
	if spec.Image == "" {
		spec.Image = api.GetImageNameOrDefault(service, project.Name)
	}
	hookService := types.ServiceConfig{
		Name:          service.Name,
		ContainerSpec: spec,
	}
	cfgs, err := s.getCreateConfigs(ctx, project, hookService, 0, nil, createOptions{
		// AutoRemove is intentionally false: a failed hook container is
		// retained so the operator can inspect its logs. runPreStartHook
		// removes it explicitly on success, and the orphan sweep catches
		// leftovers on the next run.
		AutoRemove:        false,
		UseNetworkAliases: true,
		// Tag the ephemeral hook container with the project/service it
		// belongs to so `compose down` and label-scoped tooling can find it.
		// HookLabel distinguishes hook containers from the real service
		// container; no container-number: tooling telling replicas apart must
		// not count hook containers.
		Labels: types.Labels{
			api.ProjectLabel: project.Name,
			api.ServiceLabel: service.Name,
			api.VersionLabel: api.ComposeVersion,
			api.HookLabel:    preStartHookType,
		},
	})
	if err != nil {
		return client.ContainerCreateResult{}, err
	}
	// Mounts inherit from the live service container: volumes_from carries
	// its anonymous and image volumes too. The hook's own mounts, already in
	// the host config from the merged spec, take precedence on shared
	// targets.
	cfgs.Host.VolumesFrom = append(cfgs.Host.VolumesFrom, ctr.ID)
	cfg := cfgs.Container
	hostCfg := cfgs.Host
	networkingConfig := cfgs.Network

	apiVersion, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return client.ContainerCreateResult{}, err
	}

	created, err := s.apiClient().ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkingConfig,
	})
	if err != nil {
		return client.ContainerCreateResult{}, err
	}

	if versions.LessThan(apiVersion, apiVersion144) {
		if err := s.connectPreStartExtraNetworks(ctx, project, service, created.ID, hostCfg.NetworkMode); err != nil {
			// AutoRemove is false; remove the container explicitly since it was
			// never started. Log failures so the orphan is at least visible.
			if _, removeErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); removeErr != nil {
				logrus.Warnf("service %q pre_start: failed to remove orphan hook container %s: %v", service.Name, created.ID, removeErr)
			}
			return client.ContainerCreateResult{}, err
		}
	}
	return created, nil
}

// connectPreStartExtraNetworks mirrors the createMobyContainer fallback path for
// older API versions: ContainerCreate only accepts one EndpointsConfig, so extra
// networks have to be attached via NetworkConnect after creation.
func (s *composeService) connectPreStartExtraNetworks(ctx context.Context, project *types.Project, service types.ServiceConfig, containerID string, primary container.NetworkMode) error {
	for _, networkKey := range service.NetworksByPriority() {
		mobyNetworkName := project.Networks[networkKey].Name
		if string(primary) == mobyNetworkName {
			continue
		}
		eps, err := createEndpointSettings(project, service, 0, networkKey, nil, true)
		if err != nil {
			return err
		}
		if _, err := s.apiClient().NetworkConnect(ctx, mobyNetworkName, client.NetworkConnectOptions{
			Container:      containerID,
			EndpointConfig: eps,
		}); err != nil {
			return err
		}
	}
	return nil
}

// removeOrphanPreStartContainers finds and force-removes any hook containers
// left behind by a previous failed run of this service's pre_start hooks.
// Containers are identified by project + service + HookLabel=pre_start.
// Removal failures are logged at warn level and do not abort the run.
func (s *composeService) removeOrphanPreStartContainers(ctx context.Context, projectName, serviceName string) error {
	f := projectFilter(projectName)
	f.Add("label", serviceFilter(serviceName))
	f.Add("label", hookFilter(preStartHookType))
	res, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return err
	}
	for _, ctr := range res.Items {
		if _, removeErr := s.apiClient().ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		}); removeErr != nil {
			logrus.Warnf("failed to remove stale pre_start hook container %s: %v", ctr.ID, removeErr)
		}
	}
	return nil
}

func waitPreStart(ctx context.Context, serviceName string, index int, waitRes client.ContainerWaitResult) error {
	// ContainerWait can deliver on Result and Error at the same instant. Two
	// races have to be closed deterministically here:
	//   1. The daemon closing a successful stream cleanly sends nil on Error
	//      AND the exit code on Result — a plain 3-case select would let Go
	//      pick the Error branch and report a spurious "wait ended" failure.
	//   2. A real transport error on Error can race with a stale Result — if
	//      the scheduler picks Result, we would silently drop the error and
	//      let the service start.
	// Loop until Result is delivered, nil-ing the Error channel after a nil
	// receive so a closed channel cannot busy-loop. After Result lands, do a
	// non-blocking check on Error so a real error still wins over Result.
	errCh := waitRes.Error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-waitRes.Result:
			select {
			case err := <-errCh:
				if err != nil {
					return err
				}
			default:
			}
			return preStartResultErr(serviceName, index, res)
		case err := <-errCh:
			if err != nil {
				return err
			}
			// nil on Error: stream closed cleanly. Disable this case so a
			// closed channel can't fire repeatedly.
			errCh = nil
		}
	}
}

func preStartResultErr(serviceName string, index int, res container.WaitResponse) error {
	if res.Error != nil {
		return fmt.Errorf("service %q pre_start[%d] wait error: %s", serviceName, index, res.Error.Message)
	}
	if res.StatusCode != 0 {
		return fmt.Errorf("service %q pre_start[%d] exited with code %d", serviceName, index, res.StatusCode)
	}
	return nil
}

// streamPreStartLogs opens the hook container's log stream in a background
// goroutine, tees output into the listener (when non-nil) and into per-stream
// tail buffers. It returns:
//   - done: closed when the goroutine exits; callers must wait on it
//   - getTail: returns the stderr-biased tail (stderr preferred, stdout fallback);
//     safe to call only after done is closed
//
// The log stream is always opened even when listener is nil, so the tail is
// populated in detached mode and can appear in error messages.
func (s *composeService) streamPreStartLogs(
	ctx context.Context,
	containerID string,
	service types.ServiceConfig,
	index int,
	listener api.ContainerEventListener,
) (<-chan struct{}, func() string) {
	done := make(chan struct{})
	tailOut := newOutputTail(hookOutputTailLines, hookOutputTailBytes)
	tailErr := newOutputTail(hookOutputTailLines, hookOutputTailBytes)
	getTail := func() string {
		if s := tailErr.String(); s != "" {
			return s
		}
		return tailOut.String()
	}

	source := fmt.Sprintf("%s pre_start[%d] ->", service.Name, index)
	logs, err := s.apiClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		if listener != nil {
			listener(api.ContainerEvent{
				Type:    api.HookEventLog,
				Source:  source,
				ID:      containerID,
				Service: service.Name,
				Line:    fmt.Sprintf("warning: could not attach pre_start log stream: %s", err),
			})
		}
		close(done)
		return done, getTail
	}
	go func() {
		defer close(done)
		defer logs.Close() //nolint:errcheck

		var wOut, wErr io.Writer
		wOut = tailOut
		wErr = tailErr

		if listener != nil {
			// stdout and stderr share one listener writer: ContainerEvent has no stream
			// field, so both appear identically in the live display.
			lw := utils.GetWriter(func(line string) {
				listener(api.ContainerEvent{
					Type:    api.HookEventLog,
					Source:  source,
					ID:      containerID,
					Service: service.Name,
					Line:    line,
				})
			})
			defer lw.Close() //nolint:errcheck
			wOut = io.MultiWriter(lw, tailOut)
			wErr = io.MultiWriter(lw, tailErr)
		}

		_, _ = stdcopy.StdCopy(wOut, wErr, logs)
	}()
	return done, getTail
}
