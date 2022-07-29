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
	"time"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	for _, config := range service.Secrets {
		secret := project.Secrets[config.Source]
		if secret.Environment == "" {
			continue
		}

		env, ok := project.Environment[secret.Environment]
		if !ok {
			return fmt.Errorf("environment variable %q required by secret %q is not set", secret.Environment, secret.Name)
		}
		b, err := createTar(env, config)
		if err != nil {
			return err
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", &b, moby.CopyToContainerOptions{
			CopyUIDGID: true,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func createTar(env string, config types.ServiceSecretConfig) (bytes.Buffer, error) {
	value := []byte(env)
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := uint32(0o400)
	if config.Mode != nil {
		mode = *config.Mode
	}

	target := config.Target
	if config.Target == "" {
		target = "/run/secrets/" + config.Source
	} else if !isUnixAbs(config.Target) {
		target = "/run/secrets/" + config.Target
	}

	header := &tar.Header{
		Name:    target,
		Size:    int64(len(value)),
		Mode:    int64(mode),
		ModTime: time.Now(),
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
