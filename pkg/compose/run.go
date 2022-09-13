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

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli"
	cmd "github.com/docker/cli/cli/command/container"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/pkg/stringid"
)

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	containerID, err := s.prepareRun(ctx, project, opts)
	if err != nil {
		return 0, err
	}

	start := cmd.NewStartOptions()
	start.OpenStdin = !opts.Detach && opts.Interactive
	start.Attach = !opts.Detach
	start.Containers = []string{containerID}

	err = cmd.RunStart(s.dockerCli, &start)
	if sterr, ok := err.(cli.StatusError); ok {
		return sterr.StatusCode, nil
	}
	return 0, err
}

func (s *composeService) prepareRun(ctx context.Context, project *types.Project, opts api.RunOptions) (string, error) {
	if err := prepareVolumes(project); err != nil { // all dependencies already checked, but might miss service img
		return "", err
	}
	service, err := project.GetService(opts.Service)
	if err != nil {
		return "", err
	}

	applyRunOptions(project, &service, opts)

	if err := s.dockerCli.In().CheckTty(opts.Interactive, service.Tty); err != nil {
		return "", err
	}

	slug := stringid.GenerateRandomID()
	if service.ContainerName == "" {
		service.ContainerName = fmt.Sprintf("%s_%s_run_%s", project.Name, service.Name, stringid.TruncateID(slug))
	}
	service.Scale = 1
	service.Restart = ""
	if service.Deploy != nil {
		service.Deploy.RestartPolicy = nil
	}
	service.CustomLabels = service.CustomLabels.
		Add(api.SlugLabel, slug).
		Add(api.OneoffLabel, "True")

	if err := s.ensureImagesExists(ctx, project, opts.QuietPull); err != nil { // all dependencies already checked, but might miss service img
		return "", err
	}
	if !opts.NoDeps {
		if err := s.waitDependencies(ctx, project, service.DependsOn); err != nil {
			return "", err
		}
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return "", err
	}
	updateServices(&service, observedState)

	created, err := s.createContainer(ctx, project, service, service.ContainerName, 1,
		opts.AutoRemove, opts.UseNetworkAliases, opts.Interactive)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func applyRunOptions(project *types.Project, service *types.ServiceConfig, opts api.RunOptions) {
	service.Tty = opts.Tty
	service.StdinOpen = opts.Interactive
	service.ContainerName = opts.Name

	if len(opts.Command) > 0 {
		service.Command = opts.Command
	}
	if len(opts.User) > 0 {
		service.User = opts.User
	}
	if len(opts.WorkingDir) > 0 {
		service.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != nil {
		service.Entrypoint = opts.Entrypoint
		if len(opts.Command) == 0 {
			service.Command = []string{}
		}
	}
	if len(opts.Environment) > 0 {
		cmdEnv := types.NewMappingWithEquals(opts.Environment)
		serviceOverrideEnv := cmdEnv.Resolve(func(s string) (string, bool) {
			v, ok := envResolver(project.Environment)(s)
			return v, ok
		}).RemoveEmpty()
		service.Environment.OverrideBy(serviceOverrideEnv)
	}
	for k, v := range opts.Labels {
		service.Labels = service.Labels.Add(k, v)
	}
}
