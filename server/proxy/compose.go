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

package proxy

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"

	composev1 "github.com/docker/compose-cli/protos/compose/v1"
)

func (p *proxy) Up(ctx context.Context, request *composev1.ComposeUpRequest) (*composev1.ComposeUpResponse, error) {
	project, err := getComposeProject(request.Files, request.WorkDir, request.ProjectName)
	if err != nil {
		return nil, err
	}
	return &composev1.ComposeUpResponse{ProjectName: project.Name}, Client(ctx).ComposeService().Up(ctx, project, true)
}

func (p *proxy) Down(ctx context.Context, request *composev1.ComposeDownRequest) (*composev1.ComposeDownResponse, error) {
	projectName := request.GetProjectName()
	if projectName == "" {
		project, err := getComposeProject(request.Files, request.WorkDir, request.ProjectName)
		if err != nil {
			return nil, err
		}
		projectName = project.Name
	}
	return &composev1.ComposeDownResponse{ProjectName: projectName}, Client(ctx).ComposeService().Down(ctx, projectName)
}

func (p *proxy) Services(ctx context.Context, request *composev1.ComposeServicesRequest) (*composev1.ComposeServicesResponse, error) {
	projectName := request.GetProjectName()
	if projectName == "" {
		project, err := getComposeProject(request.Files, request.WorkDir, request.ProjectName)
		if err != nil {
			return nil, err
		}
		projectName = project.Name
	}
	response := []*composev1.Service{}
	_, err := Client(ctx).ComposeService().Ps(ctx, projectName)
	if err != nil {
		return nil, err
	}
	/* FIXME need to create `docker service ls` command to re-introduce this feature
	for _, service := range services {
		response = append(response, &composev1.Service{
			Id:       service.ID,
			Name:     service.Name,
			Replicas: uint32(service.Replicas),
			Desired:  uint32(service.Desired),
			Ports:    service.Ports,
		})
	}*/
	return &composev1.ComposeServicesResponse{Services: response}, nil
}

func (p *proxy) Stacks(ctx context.Context, request *composev1.ComposeStacksRequest) (*composev1.ComposeStacksResponse, error) {
	stacks, err := Client(ctx).ComposeService().List(ctx, request.ProjectName)
	if err != nil {
		return nil, err
	}
	response := []*composev1.Stack{}
	for _, stack := range stacks {
		response = append(response, &composev1.Stack{
			Id:     stack.ID,
			Name:   stack.Name,
			Status: stack.Status,
			Reason: stack.Reason,
		})
	}
	return &composev1.ComposeStacksResponse{Stacks: response}, nil
}

func getComposeProject(files []string, workingDir string, projectName string) (*types.Project, error) {
	options, err := cli.NewProjectOptions(files, cli.WithWorkingDirectory(workingDir), cli.WithName(projectName))
	if err != nil {
		return nil, err
	}
	return cli.ProjectFromOptions(options)
}
