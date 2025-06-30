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
	"bytes"
	"errors"
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
)

// --use-api-socket is not actually supported by the Docker Engine
// but is a client-side hack (see https://github.com/docker/cli/blob/master/cli/command/container/create.go#L246)
// we replicate here by transforming the project model

func (s *composeService) useAPISocket(project *types.Project) (*types.Project, error) {
	useAPISocket := false
	for _, service := range project.Services {
		if service.UseAPISocket {
			useAPISocket = true
			break
		}
	}
	if !useAPISocket {
		return project, nil
	}

	if s.dockerCli.ServerInfo().OSType == "windows" {
		return nil, errors.New("use_api_socket can't be used with a Windows Docker Engine")
	}

	creds, err := s.dockerCli.ConfigFile().GetAllCredentials()
	if err != nil {
		return nil, fmt.Errorf("resolving credentials failed: %w", err)
	}
	newConfig := &configfile.ConfigFile{
		AuthConfigs: creds,
	}
	var configBuf bytes.Buffer
	if err := newConfig.SaveToWriter(&configBuf); err != nil {
		return nil, fmt.Errorf("saving creds for API socket: %w", err)
	}

	project.Configs["#apisocket"] = types.ConfigObjConfig{
		Content: configBuf.String(),
	}

	for name, service := range project.Services {
		if !service.UseAPISocket {
			continue
		}
		service.Volumes = append(service.Volumes, types.ServiceVolumeConfig{
			Type:   types.VolumeTypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})

		_, envvarPresent := service.Environment["DOCKER_CONFIG"]

		// If the DOCKER_CONFIG env var is already present, we assume the client knows
		// what they're doing and don't inject the creds.
		if !envvarPresent {
			// Set our special little location for the config file.
			path := "/run/secrets/docker"
			service.Environment["DOCKER_CONFIG"] = &path
		}

		service.Configs = append(service.Configs, types.ServiceConfigObjConfig{
			Source: "#apisocket",
			Target: "/run/secrets/docker/config.json",
		})
		project.Services[name] = service
	}
	return project, nil
}
