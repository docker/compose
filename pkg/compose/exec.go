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
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command/container"
	cliopts "github.com/docker/cli/opts"
	"github.com/docker/compose/v2/pkg/api"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) Exec(ctx context.Context, project string, opts api.RunOptions) (int, error) {
	target, err := s.getExecTarget(ctx, project, opts)
	if err != nil {
		return 0, err
	}

	env := cliopts.NewListOpts(nil)
	for _, s := range opts.Environment {
		env.Set(s) // nolint:errcheck - ListOpts has no validator
	}

	err = container.RunExec(s.dockerCli(), container.ExecOptions{
		Interactive: true,
		Tty:         opts.Tty,
		Detach:      opts.Detach,
		User:        opts.User,
		Privileged:  opts.Privileged,
		Env:         env,
		EnvFile:     cliopts.NewListOpts(nil),
		Workdir:     opts.WorkingDir,
		Container:   target.ID,
		Command:     opts.Command,
	})
	if sterr, ok := err.(cli.StatusError); ok {
		return sterr.StatusCode, nil
	}
	return 0, err
}

func (s *composeService) getExecTarget(ctx context.Context, projectName string, opts api.RunOptions) (moby.Container, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
			serviceFilter(opts.Service),
			containerNumberFilter(opts.Index),
		),
	})
	if err != nil {
		return moby.Container{}, err
	}
	if len(containers) < 1 {
		return moby.Container{}, fmt.Errorf("service %q is not running container #%d", opts.Service, opts.Index)
	}
	container := containers[0]
	return container, nil
}
