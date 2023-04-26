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
	"os"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	for _, config := range service.Secrets {
		secret := project.Secrets[config.Source]
		var data []byte
		switch {
		case secret.File != "":
			content, err := os.ReadFile(secret.File)
			if err != nil {
				return errors.Wrapf(err, "can't read data from secret file %s", secret.File)
			}
			data = content

		case secret.Environment != "":
			env, ok := project.Environment[secret.Environment]
			if !ok {
				return fmt.Errorf("environment variable %q required by secret %q is not set", secret.Environment, secret.Name)
			}
			data = []byte(env)
		}

		b, err := createTar(data, config)
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

func createTar(value []byte, config types.ServiceSecretConfig) (bytes.Buffer, error) {
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := uint32(0o400)
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
		Name:    getTarget(config),
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

// prepareSecrets converts secret into a bind mount when we don't have an better option
func prepareSecrets(project *types.Project) error {
	for i, service := range project.Services {
		var secrets []types.ServiceSecretConfig
		for _, config := range service.Secrets {
			secret := project.Secrets[config.Source]
			if secret.File == "" || secret.External.External {
				secrets = append(secrets, config)
				continue
			}

			stat, err := os.Stat(secret.File)
			if err != nil {
				return err
			}
			if !stat.IsDir() {
				// secret files will be injected by value, so we can set UID/GID
				// see pkg/compose/secrets.go#injectSecrets
				secrets = append(secrets, config)
				continue
			}
			if config.GID != "" || config.UID != "" {
				logrus.Warnf("secret UID/GID can't be set when secret is a directory")
			}

			service.Volumes = append(service.Volumes, types.ServiceVolumeConfig{
				Type:     types.VolumeTypeBind,
				Source:   secret.File,
				Target:   getTarget(config),
				ReadOnly: true,
			})
		}
		service.Secrets = secrets
		project.Services[i] = service
	}
	return nil
}

func getTarget(config types.ServiceSecretConfig) string {
	target := config.Target
	if config.Target == "" {
		target = "/run/secrets/" + config.Source
	} else if !isUnixAbs(config.Target) {
		target = "/run/secrets/" + config.Target
	}
	return target
}
