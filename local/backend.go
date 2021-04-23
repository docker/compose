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

	"github.com/docker/cli/cli/command"
	cliconfig "github.com/docker/cli/cli/config"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/docker/client"

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	cliopts "github.com/docker/compose-cli/cli/options"
	local_compose "github.com/docker/compose-cli/local/compose"
)

type local struct {
	containerService *containerService
	volumeService    *volumeService
	composeService   compose.Service
}

// NewService build a backend for "local" context, using Docker API client
func NewService(apiClient client.APIClient) backend.Service {
	file := cliconfig.LoadDefaultConfigFile(os.Stderr)
	return &local{
		containerService: &containerService{apiClient},
		volumeService:    &volumeService{apiClient},
		composeService:   local_compose.NewComposeService(apiClient, file),
	}
}

// GetLocalBackend initialize local backend
func GetLocalBackend(configDir string, opts cliopts.GlobalOpts) (backend.Service, error) {
	configFile, err := cliconfig.Load(configDir)
	if err != nil {
		return nil, err
	}
	options := cliflags.CommonOptions{
		Context:  opts.Context,
		Debug:    opts.Debug,
		Hosts:    opts.Hosts,
		LogLevel: opts.LogLevel,
	}

	if opts.TLSVerify {
		options.TLS = opts.TLS
		options.TLSVerify = opts.TLSVerify
		options.TLSOptions = opts.TLSOptions
	}
	apiClient, err := command.NewAPIClientFromFlags(&options, configFile)
	if err != nil {
		return nil, err
	}
	return NewService(apiClient), nil
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
