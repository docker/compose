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
	"os"

	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/client"

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/cloud"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	local_compose "github.com/docker/compose-cli/local/compose"
)

const backendType = store.EcsLocalSimulationContextType

func init() {
	backend.Register(backendType, backendType, service, getCloudService)
}

type ecsLocalSimulation struct {
	moby    *client.Client
	compose compose.Service
}

func service() (backend.Service, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &ecsLocalSimulation{
		moby:    apiClient,
		compose: local_compose.NewComposeService(apiClient, cliconfig.LoadDefaultConfigFile(os.Stderr)),
	}, nil
}

func getCloudService() (cloud.Service, error) {
	return ecsLocalSimulation{}, nil
}

func (e ecsLocalSimulation) ContainerService() containers.Service {
	return nil
}

func (e ecsLocalSimulation) VolumeService() volumes.Service {
	return nil
}

func (e ecsLocalSimulation) SecretsService() secrets.Service {
	return nil
}

func (e ecsLocalSimulation) ComposeService() compose.Service {
	return e
}

func (e ecsLocalSimulation) ResourceService() resources.Service {
	return nil
}
