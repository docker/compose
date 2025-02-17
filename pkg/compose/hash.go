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
	"encoding/json"
	"fmt"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/opencontainers/go-digest"
)

// ServiceHash computes the configuration hash for a service.
func ServiceHash(o types.ServiceConfig) (string, error) {
	// remove the Build config when generating the service hash
	o.Build = nil
	o.PullPolicy = ""
	o.Scale = nil
	if o.Deploy != nil {
		o.Deploy.Replicas = nil
	}
	o.DependsOn = nil
	o.Profiles = nil

	data, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(data).Encoded(), nil
}

// ServiceConfigsHash computes the configuration hash for service configs.
func ServiceConfigsHash(project *types.Project, serviceConfig types.ServiceConfig) (map[string]string, error) {
	serviceNameToHash := make(map[string]string)
	for _, config := range serviceConfig.Configs {
		file := project.Configs[config.Source]
		b, err := createTarForConfig(project, types.FileReferenceConfig(config), types.FileObjectConfig(file))
		if err != nil {
			return nil, err
		}

		serviceNameToHash[config.Target] = digest.SHA256.FromBytes(b.Bytes()).Encoded()
	}

	return serviceNameToHash, nil
}

// ServiceSecretsHash computes the configuration hash for service secrets.
func ServiceSecretsHash(project *types.Project, serviceConfig types.ServiceConfig) (map[string]string, error) {
	serviceNameToHash := make(map[string]string)
	for _, secret := range serviceConfig.Secrets {
		file := project.Secrets[secret.Source]
		b, err := createTarForConfig(project, types.FileReferenceConfig(secret), types.FileObjectConfig(file))
		if err != nil {
			return nil, err
		}

		serviceNameToHash[secret.Target] = digest.SHA256.FromBytes(b.Bytes()).Encoded()
	}

	return serviceNameToHash, nil
}

func createTarForConfig(
	project *types.Project,
	serviceConfig types.FileReferenceConfig,
	file types.FileObjectConfig,
) (*bytes.Buffer, error) {
	// fixed time to ensure the tarball is deterministic
	modTime := time.Unix(0, 0)

	if serviceConfig.Target == "" {
		serviceConfig.Target = "/" + serviceConfig.Source
	}

	switch {
	case file.Content != "":
		return bytes.NewBuffer([]byte(file.Content)), nil
	case file.Environment != "":
		env, ok := project.Environment[file.Environment]
		if !ok {
			return nil, fmt.Errorf(
				"environment variable %q required by file %q is not set",
				file.Environment,
				file.Name,
			)
		}
		return bytes.NewBuffer([]byte(env)), nil
	case file.File != "":
		return utils.CreateTarByPath(file.File, modTime)
	}

	return nil, fmt.Errorf("config %q is empty", file.Name)
}

// NetworkHash computes the configuration hash for a network.
func NetworkHash(o *types.NetworkConfig) (string, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(data).Encoded(), nil
}

// VolumeHash computes the configuration hash for a volume.
func VolumeHash(o types.VolumeConfig) (string, error) {
	if o.Driver == "" { // (TODO: jhrotko) This probably should be fixed in compose-go
		o.Driver = "local"
	}
	data, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(data).Encoded(), nil
}
