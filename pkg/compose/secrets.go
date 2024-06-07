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
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	for _, config := range service.Secrets {
		file := project.Secrets[config.Source]
		if file.Environment == "" {
			continue
		}

		if config.Target == "" {
			config.Target = "/run/secrets/" + config.Source
		} else if !isAbsTarget(config.Target) {
			config.Target = "/run/secrets/" + config.Target
		}

		env, ok := project.Environment[file.Environment]
		if !ok {
			return fmt.Errorf("environment variable %q required by file %q is not set", file.Environment, file.Name)
		}
		b, err := createTar(env, types.FileReferenceConfig(config))
		if err != nil {
			return err
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", &b, container.CopyToContainerOptions{
			CopyUIDGID: config.UID != "" || config.GID != "",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) injectConfigs(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	for _, config := range service.Configs {
		file := project.Configs[config.Source]
		content := file.Content
		if file.Environment != "" {
			env, ok := project.Environment[file.Environment]
			if !ok {
				return fmt.Errorf("environment variable %q required by file %q is not set", file.Environment, file.Name)
			}
			content = env
		}
		if content == "" {
			continue
		}

		if config.Target == "" {
			config.Target = "/" + config.Source
		}

		b, err := createTar(content, types.FileReferenceConfig(config))
		if err != nil {
			return err
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", &b, container.CopyToContainerOptions{
			CopyUIDGID: config.UID != "" || config.GID != "",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func createTar(env string, config types.FileReferenceConfig) (bytes.Buffer, error) {
	value := []byte(env)
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := uint32(0o444)
	if config.Mode != nil {
		mode = *config.Mode
	}

	var uid, gid int
	if config.UID != "" {
		v, err := strconv.Atoi(config.UID)
		if err != nil {
			return b, err
		}
		uid = v
	}
	if config.GID != "" {
		v, err := strconv.Atoi(config.GID)
		if err != nil {
			return b, err
		}
		gid = v
	}

	header := &tar.Header{
		Name:    config.Target,
		Size:    int64(len(value)),
		Mode:    int64(mode),
		ModTime: time.Now(),
		Uid:     uid,
		Gid:     gid,
	}
	err := tarWriter.WriteHeader(header)
	if err != nil {
		return bytes.Buffer{}, err
	}
	_, err = tarWriter.Write(value)
	if err != nil {
		return bytes.Buffer{}, err
	}
	err = tarWriter.Close()
	return b, err
}
