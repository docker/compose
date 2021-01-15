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

package local

import (
	"context"

	"github.com/docker/docker/client"

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/context/cloud"
	local_compose "github.com/docker/compose-cli/local/compose"
)

type local struct {
	containerService *containerService
	volumeService    *volumeService
	composeService   compose.Service
}

func init() {
	backend.Register("local", "local", service, cloud.NotImplementedCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &local{
		containerService: &containerService{apiClient},
		volumeService:    &volumeService{apiClient},
		composeService:   local_compose.NewComposeService(apiClient),
	}, nil
}

func (s *local) ContainerService() containers.Service {
	return s.containerService
}

func (s *local) ComposeService() compose.Service {
	return s.composeService
}

func (s *local) SecretsService() secrets.Service {
	return nil
}

func (s *local) VolumeService() volumes.Service {
	return s.volumeService
}

func (s *local) ResourceService() resources.Service {
	return nil
}
