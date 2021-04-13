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

	"github.com/docker/compose-cli/api/compose"

	"github.com/compose-spec/compose-go/types"
	apitypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	moby "github.com/docker/docker/pkg/stringid"
)

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return 0, err
	}
	containerState := NewContainersState(observedState)
	ctx = context.WithValue(ctx, ContainersKey{}, containerState)

	service, err := project.GetService(opts.Service)
	if err != nil {
		return 0, err
	}

	applyRunOptions(&service, opts)

	slug := moby.GenerateRandomID()
	if service.ContainerName == "" {
		service.ContainerName = fmt.Sprintf("%s_%s_run_%s", project.Name, service.Name, moby.TruncateID(slug))
	}
	service.Scale = 1
	service.StdinOpen = true
	service.Labels = service.Labels.Add(slugLabel, slug)
	service.Labels = service.Labels.Add(oneoffLabel, "True")

	if err := s.ensureImagesExists(ctx, project, observedState, false); err != nil { // all dependencies already checked, but might miss service img
		return 0, err
	}
	if err := s.waitDependencies(ctx, project, service); err != nil {
		return 0, err
	}
	if err := s.createContainer(ctx, project, service, service.ContainerName, 1, opts.AutoRemove, opts.UseNetworkAliases); err != nil {
		return 0, err
	}
	containerID := service.ContainerName

	if opts.Detach {
		err := s.apiClient.ContainerStart(ctx, containerID, apitypes.ContainerStartOptions{})
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(opts.Writer, containerID)
		return 0, nil
	}

	containers, err := s.apiClient.ContainerList(ctx, apitypes.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", slugLabel, slug)),
		),
		All: true,
	})
	if err != nil {
		return 0, err
	}
	oneoffContainer := containers[0]
	restore, err := s.attachContainerStreams(ctx, oneoffContainer.ID, service.Tty, opts.Reader, opts.Writer)
	if err != nil {
		return 0, err
	}
	defer restore()

	err = s.apiClient.ContainerStart(ctx, containerID, apitypes.ContainerStartOptions{})
	if err != nil {
		return 0, err
	}

	err = s.monitorTTySize(ctx, containerID, s.apiClient.ContainerResize)
	if err != nil {
		return 0, err
	}

	statusC, errC := s.apiClient.ContainerWait(context.Background(), oneoffContainer.ID, container.WaitConditionNotRunning)
	select {
	case status := <-statusC:
		return int(status.StatusCode), nil
	case err := <-errC:
		return 0, err
	}

}

func applyRunOptions(service *types.ServiceConfig, opts compose.RunOptions) {
	service.Tty = opts.Tty
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
	if len(opts.Entrypoint) > 0 {
		service.Entrypoint = opts.Entrypoint
	}
	if len(opts.Environment) > 0 {
		service.Environment.OverrideBy(opts.EnvironmentMap())
	}
	for k, v := range opts.Labels {
		service.Labels.Add(k, v)
	}
}
