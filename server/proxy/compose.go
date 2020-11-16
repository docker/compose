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
	"github.com/docker/compose-cli/api/containers"
	composev1 "github.com/docker/compose-cli/protos/compose/v1"
	containersv1 "github.com/docker/compose-cli/protos/containers/v1"
	"github.com/docker/compose-cli/server/proxy/streams"
)

func (p *proxy) Up(ctx context.Context, request *composev1.ComposeUpRequest) (*composev1.ComposeUpResponse, error) {
	options, err := cli.NewProjectOptions(request.Files,
		cli.WithOsEnv,
		cli.WithWorkingDirectory(request.WorkDir),
		cli.WithName(request.ProjectName))
	if err != nil {
		return nil, err
	}

	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return nil, err
	}

	return &composev1.ComposeUpResponse{}, Client(ctx).ComposeService().Up(ctx, project, true)
}

func (p *proxy) Down(ctx context.Context, request *composev1.ComposeDownRequest) (*composev1.ComposeDownResponse, error) {
	err := Client(ctx).ComposeService().Down(ctx, "TODO")
	if err != nil {
		return nil, err
	}
	response := &composev1.ComposeDownResponse{}
	return response, err
}

func (p *proxy) ComposeLogs(request *containersv1.LogsRequest, stream containersv1.Containers_LogsServer) error {
	return Client(stream.Context()).ContainerService().Logs(stream.Context(), request.GetContainerId(), containers.LogsRequest{
		Follow: request.Follow,
		Writer: &streams.Log{
			Stream: stream,
		},
	})
}
