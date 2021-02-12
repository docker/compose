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
	originalServices := project.Services
	var requestedService types.ServiceConfig
	for _, service := range originalServices {
		if service.Name == opts.Service {
			requestedService = service
		}
	}

	project.Services = originalServices
	if len(opts.Command) > 0 {
		requestedService.Command = opts.Command
	}
	requestedService.Scale = 1
	requestedService.Tty = true
	requestedService.StdinOpen = true

	slug := moby.GenerateRandomID()
	requestedService.ContainerName = fmt.Sprintf("%s_%s_run_%s", project.Name, requestedService.Name, moby.TruncateID(slug))
	requestedService.Labels = requestedService.Labels.Add(slugLabel, slug)
	requestedService.Labels = requestedService.Labels.Add(oneoffLabel, "True")

	if err := s.ensureImagesExists(ctx, project); err != nil { // all dependencies already checked, but might miss requestedService img
		return 0, err
	}
	if err := s.waitDependencies(ctx, project, requestedService); err != nil {
		return 0, err
	}
	if err := s.createContainer(ctx, project, requestedService, requestedService.ContainerName, 1, opts.AutoRemove); err != nil {
		return 0, err
	}
	containerID := requestedService.ContainerName

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
	err = s.attachContainerStreams(ctx, oneoffContainer, true, opts.Reader, opts.Writer)
	if err != nil {
		return 0, err
	}

	err = s.apiClient.ContainerStart(ctx, containerID, apitypes.ContainerStartOptions{})
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
