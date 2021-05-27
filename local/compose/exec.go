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

	"github.com/compose-spec/compose-go/types"
	apitypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	"github.com/docker/compose-cli/api/compose"
)

func (s *composeService) Exec(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	service, err := project.GetService(opts.Service)
	if err != nil {
		return 0, err
	}

	containers, err := s.apiClient.ContainerList(ctx, apitypes.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			serviceFilter(service.Name),
			filters.Arg("label", fmt.Sprintf("%s=%d", containerNumberLabel, opts.Index)),
		),
	})
	if err != nil {
		return 0, err
	}
	if len(containers) < 1 {
		return 0, fmt.Errorf("container %s not running", getContainerName(project.Name, service, opts.Index))
	}
	container := containers[0]

	var env []string
	for k, v := range service.Environment.OverrideBy(types.NewMappingWithEquals(opts.Environment)).
		Resolve(func(s string) (string, bool) {
			v, ok := project.Environment[s]
			return v, ok
		}).
		RemoveEmpty() {
		env = append(env, fmt.Sprintf("%s=%s", k, *v))
	}

	exec, err := s.apiClient.ContainerExecCreate(ctx, container.ID, apitypes.ExecConfig{
		Cmd:        opts.Command,
		Env:        env,
		User:       opts.User,
		Privileged: opts.Privileged,
		Tty:        opts.Tty,
		Detach:     opts.Detach,
		WorkingDir: opts.WorkingDir,

		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return 0, err
	}

	if opts.Detach {
		return 0, s.apiClient.ContainerExecStart(ctx, exec.ID, apitypes.ExecStartCheck{
			Detach: true,
			Tty:    opts.Tty,
		})
	}

	resp, err := s.apiClient.ContainerExecAttach(ctx, exec.ID, apitypes.ExecStartCheck{
		Detach: false,
		Tty:    opts.Tty,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Close()

	if opts.Tty {
		s.monitorTTySize(ctx, exec.ID, s.apiClient.ContainerExecResize)
		if err != nil {
			return 0, err
		}
	}

	readChannel := make(chan error)
	writeChannel := make(chan error)

	go func() {
		_, err := io.Copy(opts.Writer, resp.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(resp.Conn, opts.Reader)
		writeChannel <- err
	}()

	select {
	case err = <-readChannel:
		break
	case err = <-writeChannel:
		break
	}

	if err != nil {
		return 0, err
	}
	return s.getExecExitStatus(ctx, exec.ID)
}

func (s *composeService) getExecExitStatus(ctx context.Context, execID string) (int, error) {
	resp, err := s.apiClient.ContainerExecInspect(ctx, execID)
	if err != nil {
		return 0, err
	}
	return resp.ExitCode, nil
}
