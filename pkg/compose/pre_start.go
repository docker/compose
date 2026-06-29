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
	for i, hook := range service.PreStart {
		if err := s.runPreStartHook(ctx, project, service, ctr, i, hook, listener); err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) runPreStartHook(
	ctx context.Context, project *types.Project, service types.ServiceConfig,
	ctr container.Summary, index int, hook types.ServiceHook, listener api.ContainerEventListener,
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

	// Open the log stream before ContainerStart so AutoRemove cannot race us
	// to a 404 on a fast-exiting hook. The dedicated logCtx lets us force the
	// follow stream closed once the hook has exited, so a daemon that keeps
	// the connection open cannot deadlock `<-logsDone`.
	logCtx, cancelLogs := context.WithCancel(ctx)
	defer cancelLogs()
	logsDone := s.streamPreStartLogs(logCtx, created.ID, service, index, listener)

	if _, err := s.apiClient().ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		// AutoRemove only fires after a successful start, so the never-started
		// container has to be dropped explicitly. A failed removal is logged
		// at warn level — without that hint the orphan is only discoverable
		// via the project/service labels.
		if _, removeErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
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
	return waitErr
}

func (s *composeService) createPreStartContainer(
	ctx context.Context, project *types.Project, service types.ServiceConfig,
	ctr container.Summary, hook types.ServiceHook,
) (client.ContainerCreateResult, error) {
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
		// to so a failed AutoRemove leaves something that `compose down` (and
		// other label-scoped tooling) can still find.
		Labels: map[string]string{
			api.ProjectLabel: project.Name,
			api.ServiceLabel: service.Name,
			api.VersionLabel: api.ComposeVersion,
		},
	}
	hostCfg := &container.HostConfig{
		AutoRemove:  true,
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
		Config:           cfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkingConfig,
	})
	if err != nil {
		return client.ContainerCreateResult{}, err
	}

	if versions.LessThan(apiVersion, apiVersion144) {
		if err := s.connectPreStartExtraNetworks(ctx, project, service, created.ID, networkMode); err != nil {
			// Same reason as the ContainerStart-failure cleanup: AutoRemove never
			// fires on a container that was created but not started. Surface
			// any cleanup failure so the orphan is at least visible in logs.
			if _, removeErr := s.apiClient().ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
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

// streamPreStartLogs returns a channel that is closed once the hook log stream
// has been fully drained (or never opened). Callers must wait on it before
// returning so the goroutine cannot outlive the hook.
func (s *composeService) streamPreStartLogs(ctx context.Context, containerID string, service types.ServiceConfig, index int, listener api.ContainerEventListener) <-chan struct{} {
	done := make(chan struct{})
	if listener == nil {
		close(done)
		return done
	}
	source := fmt.Sprintf("%s pre_start[%d] ->", service.Name, index)
	logs, err := s.apiClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		listener(api.ContainerEvent{
			Type:    api.HookEventLog,
			Source:  source,
			ID:      containerID,
			Service: service.Name,
			Line:    fmt.Sprintf("warning: could not attach pre_start log stream: %s", err),
		})
		close(done)
		return done
	}
	go func() {
		defer close(done)
		defer logs.Close() //nolint:errcheck
		w := utils.GetWriter(func(line string) {
			listener(api.ContainerEvent{
				Type:    api.HookEventLog,
				Source:  source,
				ID:      containerID,
				Service: service.Name,
				Line:    line,
			})
		})
		defer w.Close() //nolint:errcheck
		_, _ = stdcopy.StdCopy(w, w, logs)
	}()
	return done
}
