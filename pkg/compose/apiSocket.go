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
	clitypes "github.com/docker/cli/cli/config/types"
)

// --use-api-socket is not actually supported by the Docker Engine
// but is a client-side hack (see https://github.com/docker/cli/blob/master/cli/command/container/create.go#L246)
// we replicate here by transforming the project model

// apiSocketConfigName names the config useAPISocket adds to the project, which
// is also what tells that a project went through it.
const apiSocketConfigName = "#apisocket"

// useAPISocket mounts the Docker API socket in the services asking for it and
// hands them the registry credentials of the host, the way the Docker CLI does
// for `docker run --use-api-socket`:
//   - to the container's root only: the config file is created with mode 0600
//     and the ownership the engine gives a file it copies in (root), so another
//     user of the container cannot read it;
//   - not to a service that sets DOCKER_CONFIG itself, which is taken to know
//     where its credentials are, nor when the host has none.
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
	// The project already went through here, as the plan's own project does
	// when it is handed to a later plan (a watch rebuild): transforming it a
	// second time would append the socket volume and the config entry again,
	// duplicate entries in the model that also change the service's config hash.
	if _, applied := project.Configs[apiSocketConfigName]; applied {
		return project, nil
	}

	if s.getContextInfo().ServerOSType() == "windows" {
		return nil, errors.New("use_api_socket can't be used with a Windows Docker Engine")
	}

	// The credentials are looked up only if a service is to get them: one that
	// sets DOCKER_CONFIG is taken to know what it does.
	var creds map[string]clitypes.AuthConfig
	for _, service := range project.Services {
		if service.UseAPISocket && !definesDockerConfig(service) {
			var err error
			creds, err = s.configFile().GetAllCredentials()
			if err != nil {
				return nil, fmt.Errorf("resolving credentials failed: %w", err)
			}
			break
		}
	}

	newConfig := &configfile.ConfigFile{
		AuthConfigs: creds,
	}
	var configBuf bytes.Buffer
	if err := newConfig.SaveToWriter(&configBuf); err != nil {
		return nil, fmt.Errorf("saving creds for API socket: %w", err)
	}

	// the maps to write into may be nil: a programmatic project has no Configs,
	// and the one `compose start` loads, which resolves no environment, has no
	// service Environment
	if project.Configs == nil {
		project.Configs = types.Configs{}
	}
	// also tells that the project went through here, whether or not any service
	// is given the credentials
	project.Configs[apiSocketConfigName] = types.ConfigObjConfig{
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

		if len(creds) > 0 && !definesDockerConfig(service) {
			if service.Environment == nil {
				service.Environment = types.MappingWithEquals{}
			}
			// Set our special little location for the config file.
			path := "/run/secrets/docker"
			service.Environment["DOCKER_CONFIG"] = &path

			// like the file of the Docker CLI: readable by root only. One mode
			// per service, so that no service shares it with another.
			mode := types.FileMode(0o600)
			service.Configs = append(service.Configs, types.ServiceConfigObjConfig{
				Source: apiSocketConfigName,
				Target: "/run/secrets/docker/config.json",
				Mode:   &mode,
			})
		}
		project.Services[name] = service
	}
	return project, nil
}

// definesDockerConfig tells whether the service sets DOCKER_CONFIG itself.
func definesDockerConfig(service types.ServiceConfig) bool {
	_, defined := service.Environment["DOCKER_CONFIG"]
	return defined
}
