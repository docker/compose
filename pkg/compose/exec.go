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
	"strings"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command/container"
	"github.com/docker/compose/v2/pkg/api"
	moby "github.com/docker/docker/api/types"
)

func (s *composeService) Exec(ctx context.Context, projectName string, options api.RunOptions) (int, error) {
	projectName = strings.ToLower(projectName)
	target, err := s.getExecTarget(ctx, projectName, options)
	if err != nil {
		return 0, err
	}

	exec := container.NewExecOptions()
	exec.Interactive = options.Interactive
	exec.TTY = options.Tty
	exec.Detach = options.Detach
	exec.User = options.User
	exec.Privileged = options.Privileged
	exec.Workdir = options.WorkingDir
	exec.Container = target.ID
	exec.Command = options.Command
	for _, v := range options.Environment {
		err := exec.Env.Set(v)
		if err != nil {
			return 0, err
		}
	}

	err = container.RunExec(s.dockerCli, exec)
	if sterr, ok := err.(cli.StatusError); ok {
		return sterr.StatusCode, nil
	}
	return 0, err
}

func (s *composeService) getExecTarget(ctx context.Context, projectName string, opts api.RunOptions) (moby.Container, error) {
	return s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, opts.Service, opts.Index)
}
