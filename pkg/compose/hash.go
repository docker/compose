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
	"os"
	"path/filepath"
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

	bytes, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(bytes).Encoded(), nil
}

// ServiceConfigsHash computes the configuration hash for service configs.
func ServiceConfigsHash(project *types.Project, serviceConfig types.ServiceConfig) (string, error) {
	bytes := make([]byte, 0)
	for _, config := range serviceConfig.Configs {
		file := project.Configs[config.Source]
		b, err := createTarForConfig(project, types.FileReferenceConfig(config), types.FileObjectConfig(file))

		if err != nil {
			return "", err
		}

		bytes = append(bytes, b.Bytes()...)
	}

	return digest.SHA256.FromBytes(bytes).Encoded(), nil
}

// ServiceSecretsHash computes the configuration hash for service secrets.
func ServiceSecretsHash(project *types.Project, serviceConfig types.ServiceConfig) (string, error) {
	bytes := make([]byte, 0)
	for _, secret := range serviceConfig.Secrets {
		file := project.Secrets[secret.Source]
		b, err := createTarForConfig(project, types.FileReferenceConfig(secret), types.FileObjectConfig(file))

		if err != nil {
			return "", err
		}

		bytes = append(bytes, b.Bytes()...)
	}

	return digest.SHA256.FromBytes(bytes).Encoded(), nil
}

func createTarForConfig(
	project *types.Project,
	serviceConfig types.FileReferenceConfig,
	file types.FileObjectConfig,
) (*bytes.Buffer, error) {
	// fixed time to ensure the tarball is deterministic
	modTime := time.Unix(0, 0)
	content := make([]byte, 0)

	if file.Content != "" {
		content = []byte(file.Content)
	} else if file.Environment != "" {
		env, ok := project.Environment[file.Environment]
		if !ok {
			return nil, fmt.Errorf(
				"environment variable %q required by file %q is not set",
				file.Environment,
				file.Name,
			)
		}
		content = []byte(env)
	} else if file.File != "" {
		var err error
		content, err = readPathContent(file.File)

		if err != nil {
			return nil, err
		}
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("config %q is empty", file.Name)
	}

	if serviceConfig.Target == "" {
		serviceConfig.Target = "/" + serviceConfig.Source
	}

	b, err := utils.CreateTar(content, serviceConfig, modTime)
	if err != nil {
		return nil, err
	}

	return &b, nil
}

func readPathContent(path string) ([]byte, error) {
	content := make([]byte, 0)

	// Check if the path is a directory
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error accessing path %q: %v", path, err)
	}

	if info.IsDir() {
		// If it's a directory, read all files and concatenate their contents
		err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				fileContent, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				content = append(content, fileContent...)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("error reading directory %q: %v", path, err)
		}
	} else {
		// If it's a file, read its content
		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error reading file %q: %v", path, err)
		}
		content = fileContent
	}

	return content, nil
}
