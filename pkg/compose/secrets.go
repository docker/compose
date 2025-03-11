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
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/docker/api/types/container"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	for _, config := range service.Secrets {
		source := project.Secrets[config.Source]
		content := source.Content
		if source.Environment != "" {
			continue
		}

		if config.Target == "" {
			config.Target = "/run/secrets/" + config.Source
		} else if !isAbsTarget(config.Target) {
			config.Target = "/run/secrets/" + config.Target
		}

		var tar *bytes.Buffer
		b, err := utils.CreateTar([]byte(content), types.FileReferenceConfig(config))
		if err != nil {
			return err
		}
		tar = b

		if service.ReadOnly {
			return fmt.Errorf("cannot create secret %q in read-only service %s: `source` is the sole supported option", source.Name, service.Name)
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", tar, container.CopyToContainerOptions{
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
		source := project.Configs[config.Source]
		content := source.Content
		if source.Environment != "" {
			env, ok := project.Environment[source.Environment]
			if !ok {
				return fmt.Errorf("environment variable %q required by config %q is not set", source.Environment, source.Name)
			}
			content = env
		}
		if config.Target == "" {
			config.Target = "/" + config.Source
		}

		var tar *bytes.Buffer
		switch {
		case content != "":
			b, err := utils.CreateTar([]byte(content), types.FileReferenceConfig(config))
			if err != nil {
				return err
			}
			tar = b
		case config.UID != "", config.GID != "", config.Mode != nil:
			b, err := utils.CreateTarByFile(source.File, types.FileReferenceConfig(config))
			if err != nil {
				return err
			}
			tar = b
		default:
			// config is managed by bind mount
			continue
		}

		if service.ReadOnly {
			return fmt.Errorf("cannot create config %q in read-only service %s: `source` is the sole supported option", source.Name, service.Name)
		}

		err := s.apiClient().CopyToContainer(ctx, id, "/", tar, container.CopyToContainerOptions{
			CopyUIDGID: config.UID != "" || config.GID != "",
		})
		if err != nil {
			return err
		}
	}
	return nil
}
