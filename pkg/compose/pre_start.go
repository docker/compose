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
	"strings"

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

// getHookContainerName builds the deterministic name of a pre_start runner
// container, e.g. "myproject-db-pre_start-0". A stable name makes the runner
// an addressable resource of the reconciliation plan: repeated plans converge
// on the same container instead of accumulating anonymous ones.
func getHookContainerName(projectName, serviceName string, index int) string {
	return strings.Join([]string{projectName, serviceName, preStartHookType, strconv.Itoa(index)}, api.Separator)
}

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
// order. Each hook runs in a runner container prepared by the reconciliation
// plan (see planPreStartHookRunners) that shares the service container's
// volumes via VolumesFrom and is attached to the same networks. A non-zero
// exit gates service start.
//
// runPreStart never creates a runner itself: a declared hook without a fresh
// runner is an error telling the user to reconcile — the runners were either
// consumed by a previous start (e.g. `stop` then `start`) or never prepared.
//
// With per_replica: false (the only currently supported mode), the hook sees
// the volumes of the first non-running replica only — anonymous volumes and
// tmpfs mounts are per-replica and not shared. Use named volumes or bind
// mounts for data the hook produces.
func (s *composeService) runPreStart(ctx context.Context, project *types.Project, service types.ServiceConfig, listener api.ContainerEventListener) error {
	// Validate every hook up front so an unsupported entry never triggers any I/O.
	for i, hook := range service.PreStart {
		if hook.PerReplica {
			return fmt.Errorf("service %q pre_start[%d]: per_replica is not yet supported; remove per_replica or set it to false", service.Name, i)
		}
	}
	runners, err := s.listPreStartRunners(ctx, project.Name, service.Name)
	if err != nil {
		return err
	}
	for i := range service.PreStart {
		runner, ok := runners[i]
		if !ok {
			return fmt.Errorf("service %q pre_start[%d]: no hook runner container found — runners are prepared when the service is created and consumed when its hooks run; run %q to prepare them again",
				service.Name, i, "docker compose up "+service.Name)
		}
		if err := s.execPreStartHook(ctx, service, i, runner.ID, listener); err != nil {
			return err
		}
		// Success: remove the hook container, mirroring the old AutoRemove behaviour
		// (including its anonymous volumes). A removal failure is logged but does not
		// gate service start — the hook already succeeded.
		if _, removeErr := s.apiClient().ContainerRemove(ctx, runner.ID, client.ContainerRemoveOptions{RemoveVolumes: true}); removeErr != nil {
			logrus.Warnf("service %q pre_start[%d]: failed to remove hook container %s: %v", service.Name, i, runner.ID, removeErr)
		}
	}
	return nil
}

// listPreStartRunners returns the service's created-state pre_start hook
// runner containers, indexed by their HookIndexLabel. Runners in any other
// state are ignored — a failed hook retained for inspection or an
// old-generation container without an index label is not executable; only a
// fresh runner prepared by the reconciliation plan is.
func (s *composeService) listPreStartRunners(ctx context.Context, projectName, serviceName string) (map[int]container.Summary, error) {
	f := projectFilter(projectName)
	f.Add("label", serviceFilter(serviceName))
	f.Add("label", hookFilter(preStartHookType))
	res, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, err
	}
	runners := map[int]container.Summary{}
	for _, ctr := range res.Items {
		if ctr.State != container.StateCreated {
			continue
		}
		index, err := strconv.Atoi(ctr.Labels[api.HookIndexLabel])
		if err != nil {
			continue
		}
		runners[index] = ctr
	}
	return runners, nil
}

// execPreStartHook starts an already-created hook container, streams its logs
// and waits for its exit. It owns only execution-failure handling: a container
// that never started or a run cancelled by the user is removed, a genuinely
// failed hook is retained for post-mortem inspection. Removing the container
// after a successful run is the caller's job — the runner was prepared by the
// reconciliation plan and is consumed (removed) by runPreStart on success.
func (s *composeService) execPreStartHook(
	ctx context.Context, service types.ServiceConfig,
	index int, containerID string, listener api.ContainerEventListener,
) error {
	// Subscribe to wait before start to avoid missing the exit event for short-lived hooks.
	// WaitConditionNotRunning would match immediately because the container is still in
	// "created" state, so use WaitConditionNextExit to block until the run actually finishes.
	waitRes := s.apiClient().ContainerWait(ctx, containerID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNextExit,
	})

	// Open the log stream before ContainerStart so a fast-exiting hook cannot
	// race us to a 404. The dedicated logCtx lets us force the follow stream
	// closed once the hook has exited, so a daemon that keeps the connection
	// open cannot deadlock `<-logsDone`.
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()
	logsDone, getTail := s.streamPreStartLogs(logCtx, containerID, service, index, listener)

	if _, err := s.apiClient().ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		// AutoRemove is false, so we must remove the never-started container
		// explicitly. A failed removal is logged so the orphan is visible.
		if _, removeErr := s.apiClient().ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); removeErr != nil {
			logrus.Warnf("service %q pre_start[%d]: failed to remove orphan hook container %s: %v", service.Name, index, containerID, removeErr)
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
			if _, removeErr := s.apiClient().ContainerRemove(context.Background(), containerID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); removeErr != nil {
				logrus.Warnf("service %q pre_start[%d]: failed to remove hook container %s after cancellation: %v", service.Name, index, containerID, removeErr)
			}
			return waitErr
		}
		// Genuine hook failure: retain the container so the operator can run
		// `docker logs <id>` and `docker inspect <id>` to diagnose the failure.
		// Include the short container ID in the error to make it actionable.
		shortID := containerID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		if tail := getTail(); tail != "" {
			return fmt.Errorf("%w: %s (hook container %s retained for inspection)", waitErr, tail, shortID)
		}
		return fmt.Errorf("%w (hook container %s retained for inspection)", waitErr, shortID)
	}
	return nil
}

// createPreStartContainer creates the runner container for the index-th
// pre_start hook of service, named after getHookContainerName and stamped
// with the hook labels so both the start phase (by index) and the purge (by
// hook type) can find it. It only creates: the runner is left in created
// state for the start phase to execute.
func (s *composeService) createPreStartContainer(
	ctx context.Context, project *types.Project, service types.ServiceConfig,
	ctr container.Summary, index int, name string,
) (client.ContainerCreateResult, error) {
	hook := service.PreStart[index]
	image := hook.Image
	if image == "" {
		image = api.GetImageNameOrDefault(service, project.Name)
	}

	cfg := &container.Config{
		Image:      image,
		Cmd:        hook.Command,
		User:       hook.User,
		WorkingDir: hook.WorkingDir,
		Env:        append(ToMobyEnv(service.Environment), ToMobyEnv(hook.Environment)...),
		// Tag the ephemeral hook container with the project/service it belongs
		// to so it can be found by `compose down` and label-scoped tooling.
		// HookLabel also distinguishes hook containers from the real service
		// container (which shares ProjectLabel and ServiceLabel).
		Labels: map[string]string{
			api.ProjectLabel:   project.Name,
			api.ServiceLabel:   service.Name,
			api.VersionLabel:   api.ComposeVersion,
			api.HookLabel:      preStartHookType,
			api.HookIndexLabel: strconv.Itoa(index),
		},
	}
	hostCfg := &container.HostConfig{
		// AutoRemove is intentionally false: a failed hook container is retained
		// so the operator can inspect its logs. On success runPreStartHook
		// removes the container explicitly (see the success path below).
		AutoRemove:  false,
		Privileged:  hook.Privileged,
		VolumesFrom: []string{ctr.ID},
	}

	apiVersion, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return client.ContainerCreateResult{}, err
	}

	networkMode, networkingConfig, err := defaultNetworkSettings(project, service, 0, nil, true, apiVersion)
	if err != nil {
		return client.ContainerCreateResult{}, err
	}
	hostCfg.NetworkMode = networkMode

	created, err := s.apiClient().ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:             name,
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkingConfig,
	})
	if err != nil {
		return client.ContainerCreateResult{}, err
	}

	if versions.LessThan(apiVersion, apiVersion144) {
		if err := s.connectPreStartExtraNetworks(ctx, project, service, created.ID, networkMode); err != nil {
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
