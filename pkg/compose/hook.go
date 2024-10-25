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
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

func (s composeService) runHook(ctx context.Context, container moby.Container, service types.ServiceConfig, hook types.ServiceHook, listener api.ContainerEventListener) error {
	wOut := utils.GetWriter(func(line string) {
		listener(api.ContainerEvent{
			Type:      api.HookEventLog,
			Container: getContainerNameWithoutProject(container) + " ->",
			ID:        container.ID,
			Service:   service.Name,
			Line:      line,
		})
	})
	defer wOut.Close() //nolint:errcheck

	detached := listener == nil
	exec, err := s.apiClient().ContainerExecCreate(ctx, container.ID, containerType.ExecOptions{
		User:         hook.User,
		Privileged:   hook.Privileged,
		Env:          ToMobyEnv(hook.Environment),
		WorkingDir:   hook.WorkingDir,
		Cmd:          hook.Command,
		Detach:       detached,
		AttachStdout: !detached,
		AttachStderr: !detached,
	})
	if err != nil {
		return err
	}

	if detached {
		return s.runWaitExec(ctx, exec, service, listener)
	}

	height, width := s.stdout().GetTtySize()
	consoleSize := &[2]uint{height, width}
	attach, err := s.apiClient().ContainerExecAttach(ctx, exec.ID, containerType.ExecAttachOptions{
		Tty:         service.Tty,
		ConsoleSize: consoleSize,
	})
	if err != nil {
		return err
	}
	defer attach.Close()

	if service.Tty {
		_, err = io.Copy(wOut, attach.Reader)
	} else {
		_, err = stdcopy.StdCopy(wOut, wOut, attach.Reader)
	}
	if err != nil {
		return err
	}

	inspected, err := s.apiClient().ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return err
	}
	if inspected.ExitCode != 0 {
		return fmt.Errorf("%s hook exited with status %d", service.Name, inspected.ExitCode)
	}
	return nil
}

func (s composeService) runWaitExec(ctx context.Context, exec moby.IDResponse, service types.ServiceConfig, listener api.ContainerEventListener) error {
	err := s.apiClient().ContainerExecStart(ctx, exec.ID, containerType.ExecStartOptions{
		Detach: listener == nil,
		Tty:    service.Tty,
	})
	if err != nil {
		return nil
	}

	// We miss a ContainerExecWait API
	tick := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			inspect, err := s.apiClient().ContainerExecInspect(ctx, exec.ID)
			if err != nil {
				return nil
			}
			if !inspect.Running {
				if inspect.ExitCode != 0 {
					return fmt.Errorf("%s hook exited with status %d", service.Name, inspect.ExitCode)
				}
				return nil
			}
		}
	}
}
