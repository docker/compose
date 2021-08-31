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
	"github.com/docker/cli/cli/streams"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/term"

	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Exec(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	service, err := project.GetService(opts.Service)
	if err != nil {
		return 0, err
	}

	container, err := s.getExecTarget(ctx, project, service, opts)
	if err != nil {
		return 0, err
	}

	exec, err := s.apiClient.ContainerExecCreate(ctx, container.ID, moby.ExecConfig{
		Cmd:        opts.Command,
		Env:        s.getExecEnvironment(project, service, opts),
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
		return 0, s.apiClient.ContainerExecStart(ctx, exec.ID, moby.ExecStartCheck{
			Detach: true,
			Tty:    opts.Tty,
		})
	}

	resp, err := s.apiClient.ContainerExecAttach(ctx, exec.ID, moby.ExecStartCheck{
		Tty: opts.Tty,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Close() //nolint:errcheck

	if opts.Tty {
		s.monitorTTySize(ctx, exec.ID, s.apiClient.ContainerExecResize)
		if err != nil {
			return 0, err
		}
	}

	err = s.interactiveExec(ctx, opts, resp)
	if err != nil {
		return 0, err
	}

	return s.getExecExitStatus(ctx, exec.ID)
}

// inspired by https://github.com/docker/cli/blob/master/cli/command/container/exec.go#L116
func (s *composeService) interactiveExec(ctx context.Context, opts api.RunOptions, resp moby.HijackedResponse) error {
	outputDone := make(chan error)
	inputDone := make(chan error)

	stdout := ContainerStdout{HijackedResponse: resp}
	stdin := ContainerStdin{HijackedResponse: resp}
	r, err := s.getEscapeKeyProxy(opts.Stdin)
	if err != nil {
		return err
	}

	in := streams.NewIn(opts.Stdin)
	if in.IsTerminal() {
		state, err := term.SetRawTerminal(in.FD())
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(in.FD(), state) //nolint:errcheck
	}

	go func() {
		if opts.Tty {
			_, err := io.Copy(opts.Stdout, stdout)
			outputDone <- err
		} else {
			_, err := stdcopy.StdCopy(opts.Stdout, opts.Stderr, stdout)
			outputDone <- err
		}
		stdout.Close() //nolint:errcheck
	}()

	go func() {
		_, err := io.Copy(stdin, r)
		inputDone <- err
		stdin.Close() //nolint:errcheck
	}()

	for {
		select {
		case err := <-outputDone:
			return err
		case err := <-inputDone:
			if _, ok := err.(term.EscapeError); ok {
				return nil
			}
			if err != nil {
				return err
			}
			// Wait for output to complete streaming
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *composeService) getExecTarget(ctx context.Context, project *types.Project, service types.ServiceConfig, opts api.RunOptions) (moby.Container, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			serviceFilter(service.Name),
			containerNumberFilter(opts.Index),
		),
	})
	if err != nil {
		return moby.Container{}, err
	}
	if len(containers) < 1 {
		return moby.Container{}, fmt.Errorf("container %s not running", getContainerName(project.Name, service, opts.Index))
	}
	container := containers[0]
	return container, nil
}

func (s *composeService) getExecEnvironment(project *types.Project, service types.ServiceConfig, opts api.RunOptions) []string {
	var env []string
	for k, v := range service.Environment.OverrideBy(types.NewMappingWithEquals(opts.Environment)).
		Resolve(func(s string) (string, bool) {
			v, ok := project.Environment[s]
			return v, ok
		}).
		RemoveEmpty() {
		env = append(env, fmt.Sprintf("%s=%s", k, *v))
	}
	return env
}

func (s *composeService) getExecExitStatus(ctx context.Context, execID string) (int, error) {
	resp, err := s.apiClient.ContainerExecInspect(ctx, execID)
	if err != nil {
		return 0, err
	}
	return resp.ExitCode, nil
}
