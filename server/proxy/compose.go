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
	project, err := getComposeProject(request.Files, request.WorkDir, request.ProjectName)
	if err != nil {
		return nil, err
	}
	return &composev1.ComposeDownResponse{ProjectName: project.Name}, Client(ctx).ComposeService().Down(ctx, project.Name)
}

func (p *proxy) ListStacks(ctx context.Context, request *composev1.ComposeDownRequest) (*composev1.ComposeDownResponse, error) {
	project, err := getComposeProject(request.Files, request.WorkDir, request.ProjectName)
	if err != nil {
		return nil, err
	}
	return &composev1.ComposeDownResponse{ProjectName: project.Name}, Client(ctx).ComposeService().Down(ctx, project.Name)
}

func getComposeProject(files []string, workingDir string, projectName string) (*types.Project, error) {
	options, err := cli.NewProjectOptions(files, cli.WithWorkingDirectory(workingDir), cli.WithName(projectName))
	if err != nil {
		return nil, err
	}
	return cli.ProjectFromOptions(options)
}
