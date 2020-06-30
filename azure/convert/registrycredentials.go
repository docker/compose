/*
   Copyright 2020 Docker, Inc.

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

package convert

import (
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"

	"github.com/docker/api/compose"
)

// Specific username from ACR docs : https://github.com/Azure/acr/blob/master/docs/AAD-OAuth.md#getting-credentials-programatically
const (
	tokenUsername = "00000000-0000-0000-0000-000000000000"
	dockerHub     = "index.docker.io"
)

type registryConfLoader interface {
	getAllRegistryCredentials() (map[string]types.AuthConfig, error)
}

type cliRegistryConfLoader struct {
	cfg *configfile.ConfigFile
}

func (c cliRegistryConfLoader) getAllRegistryCredentials() (map[string]types.AuthConfig, error) {
	return c.cfg.GetAllCredentials()
}

func newCliRegistryConfLoader() cliRegistryConfLoader {
	return cliRegistryConfLoader{
		cfg: config.LoadDefaultConfigFile(os.Stderr),
	}
}

func getRegistryCredentials(project compose.Project, registryLoader registryConfLoader) ([]containerinstance.ImageRegistryCredential, error) {
	allCreds, err := registryLoader.getAllRegistryCredentials()
	if err != nil {
		return nil, err
	}
	usedRegistries := map[string]bool{}
	for _, service := range project.Services {
		imageName := service.Image
		tokens := strings.Split(imageName, "/")
		registry := tokens[0]
		if len(tokens) == 1 { // ! image names can include "." ...
			registry = dockerHub
		} else if !strings.Contains(registry, ".") {
			registry = dockerHub
		}
		usedRegistries[registry] = true
	}
	var registryCreds []containerinstance.ImageRegistryCredential
	for name, oneCred := range allCreds {
		parsedURL, err := url.Parse(name)
		if err != nil {
			return nil, err
		}

		hostname := parsedURL.Host
		if hostname == "" {
			hostname = parsedURL.Path
		}
		if _, ok := usedRegistries[hostname]; ok {
			if oneCred.Password != "" {
				aciCredential := containerinstance.ImageRegistryCredential{
					Server:   to.StringPtr(hostname),
					Password: to.StringPtr(oneCred.Password),
					Username: to.StringPtr(oneCred.Username),
				}
				registryCreds = append(registryCreds, aciCredential)
			} else if oneCred.IdentityToken != "" {
				userName := tokenUsername
				if oneCred.Username != "" {
					userName = oneCred.Username
				}
				aciCredential := containerinstance.ImageRegistryCredential{
					Server:   to.StringPtr(hostname),
					Password: to.StringPtr(oneCred.IdentityToken),
					Username: to.StringPtr(userName),
				}
				registryCreds = append(registryCreds, aciCredential)
			}
		}
	}
	return registryCreds, nil
}
