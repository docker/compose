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

func (s *composeService) Exec(ctx context.Context, project *types.Project, opts compose.RunOptions) error {
	service, err := project.GetService(opts.Service)
	if err != nil {
		return err
	}

	containers, err := s.apiClient.ContainerList(ctx, apitypes.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			serviceFilter(service.Name),
			filters.Arg("label", fmt.Sprintf("%s=%d", containerNumberLabel, opts.Index)),
		),
	})
	if err != nil {
		return err
	}
	if len(containers) < 1 {
		return fmt.Errorf("container %s not running", getContainerName(project.Name, service, opts.Index))
	}
	container := containers[0]

	exec, err := s.apiClient.ContainerExecCreate(ctx, container.ID, apitypes.ExecConfig{
		Cmd:        opts.Command,
		Env:        opts.Environment,
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
		return err
	}

	if opts.Detach {
		return s.apiClient.ContainerExecStart(ctx, exec.ID, apitypes.ExecStartCheck{
			Detach: true,
			Tty:    opts.Tty,
		})
	}

	resp, err := s.apiClient.ContainerExecAttach(ctx, exec.ID, apitypes.ExecStartCheck{
		Detach: false,
		Tty:    opts.Tty,
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	if opts.Tty {
		err := s.monitorTTySize(ctx, exec.ID, s.apiClient.ContainerExecResize)
		if err != nil {
			return err
		}
	}

	readChannel := make(chan error, 10)
	writeChannel := make(chan error, 10)

	go func() {
		_, err := io.Copy(opts.Writer, resp.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(resp.Conn, opts.Reader)
		writeChannel <- err
	}()

	for {
		select {
		case err := <-readChannel:
			return err
		case err := <-writeChannel:
			return err
		}
	}
}
