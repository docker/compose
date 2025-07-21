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
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

func (s composeService) runHook(ctx context.Context, ctr container.Summary, service types.ServiceConfig, hook types.ServiceHook, listener api.ContainerEventListener) error {
	wOut := utils.GetWriter(func(line string) {
		listener(api.ContainerEvent{
			Type:    api.HookEventLog,
			Source:  getContainerNameWithoutProject(ctr) + " ->",
			ID:      ctr.ID,
			Service: service.Name,
			Line:    line,
		})
	})
	defer wOut.Close() //nolint:errcheck

	detached := listener == nil
	exec, err := s.apiClient().ExecCreate(ctx, ctr.ID, client.ExecCreateOptions{
		User:         hook.User,
		Privileged:   hook.Privileged,
		Env:          ToMobyEnv(hook.Environment),
		WorkingDir:   hook.WorkingDir,
		Cmd:          hook.Command,
		AttachStdout: !detached,
		AttachStderr: !detached,
	})
	if err != nil {
		return err
	}

	if detached {
		return s.runWaitExec(ctx, exec.ID, service, listener)
	}

	height, width := s.stdout().GetTtySize()
	consoleSize := client.ConsoleSize{
		Width:  width,
		Height: height,
	}
	attach, err := s.apiClient().ExecAttach(ctx, exec.ID, client.ExecAttachOptions{
		TTY:         service.Tty,
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

	inspected, err := s.apiClient().ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.ExitCode != 0 {
		return fmt.Errorf("%s hook exited with status %d", service.Name, inspected.ExitCode)
	}
	return nil
}

func (s composeService) runWaitExec(ctx context.Context, execID string, service types.ServiceConfig, listener api.ContainerEventListener) error {
	_, err := s.apiClient().ExecStart(ctx, execID, client.ExecStartOptions{
		Detach: listener == nil,
		TTY:    service.Tty,
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
			inspect, err := s.apiClient().ExecInspect(ctx, execID, client.ExecInspectOptions{})
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
