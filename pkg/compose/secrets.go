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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	moby "github.com/docker/docker/api/types/container"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	const secretsBaseDir = "/run/secrets/"
	for _, secret := range service.Secrets {
		if secret.Target == "" {
			secret.Target = path.Join(secretsBaseDir, secret.Source)
		} else if !isAbsTarget(secret.Target) {
			secret.Target = path.Join(secretsBaseDir, secret.Target)
		}

		definedSecret := project.Secrets[secret.Source]
		if definedSecret.Driver != "" {
			return errors.New("docker compose does not support secrets.*.driver")
		}
		if definedSecret.TemplateDriver != "" {
			return errors.New("docker compose does not support secrets.*.template_driver")
		}

		var tarArchive bytes.Buffer
		var err error
		switch {
		case definedSecret.External == true:
			err = fmt.Errorf("unsupported external secret %s", definedSecret.Name)
		case definedSecret.Content != "":
			tarArchive, err = createTarredFileOf(definedSecret.Content, types.FileReferenceConfig(secret))
		case definedSecret.File != "":
			tarArchive, err = createTarArchiveOf(definedSecret.File, types.FileReferenceConfig(secret))
		case definedSecret.Environment != "":
			env, ok := project.Environment[definedSecret.Environment]
			if !ok {
				return fmt.Errorf("environment variable %q required by file %q is not set", definedSecret.Environment, definedSecret.Name)
			}
			tarArchive, err = createTarredFileOf(env, types.FileReferenceConfig(secret))
		}

		if err != nil {
			return err
		}

		// secret was handled elsewhere (e.g it was external)
		if tarArchive.Len() == 0 {
			continue
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", &tarArchive, moby.CopyToContainerOptions{
			CopyUIDGID: secret.UID != "" || secret.GID != "",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) injectConfigs(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	const configsBaseDir = "/"
	for _, config := range service.Configs {
		if config.Target == "" {
			config.Target = path.Join(configsBaseDir, config.Source)
		} else if !isAbsTarget(config.Target) {
			config.Target = path.Join(configsBaseDir, config.Target)
		}

		definedConfig := project.Configs[config.Source]
		if definedConfig.Driver != "" {
			return errors.New("docker compose does not support configs.*.driver")
		}
		if definedConfig.TemplateDriver != "" {
			return errors.New("docker compose does not support configs.*.template_driver")
		}

		var tarArchive bytes.Buffer
		var err error
		switch {
		case definedConfig.External == true:
			err = fmt.Errorf("unsupported external config %s", definedConfig.Name)
		case definedConfig.File != "":
			tarArchive, err = createTarArchiveOf(definedConfig.File, types.FileReferenceConfig(config))
		case definedConfig.Content != "":
			tarArchive, err = createTarredFileOf(definedConfig.Content, types.FileReferenceConfig(config))
		case definedConfig.Environment != "":
			env, ok := project.Environment[definedConfig.Environment]
			if !ok {
				return fmt.Errorf("environment variable %q required by file %q is not set", definedConfig.Environment, definedConfig.Name)
			}
			tarArchive, err = createTarredFileOf(env, types.FileReferenceConfig(config))
		}

		if err != nil {
			return err
		}

		// config was handled elsewhere (e.g it was external)
		if tarArchive.Len() == 0 {
			continue
		}

		err = s.apiClient().CopyToContainer(ctx, id, "/", &tarArchive, moby.CopyToContainerOptions{
			CopyUIDGID: config.UID != "" || config.GID != "",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func createTarredFileOf(value string, config types.FileReferenceConfig) (bytes.Buffer, error) {
	mode, uid, gid, err := makeTarFileEntryParams(config)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("failed parsing target file parameters")
	}

	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	valueAsBytes := []byte(value)
	header := &tar.Header{
		Name:    config.Target,
		Size:    int64(len(valueAsBytes)),
		Mode:    mode,
		ModTime: time.Now(),
		Uid:     uid,
		Gid:     gid,
	}
	err = tarWriter.WriteHeader(header)
	if err != nil {
		return bytes.Buffer{}, err
	}
	_, err = tarWriter.Write(valueAsBytes)
	if err != nil {
		return bytes.Buffer{}, err
	}
	err = tarWriter.Close()
	return b, err
}

func createTarArchiveOf(path string, config types.FileReferenceConfig) (bytes.Buffer, error) {
	// need to treat files and directories differently
	fi, err := os.Stat(path)
	if err != nil {
		return bytes.Buffer{}, err
	}

	// if path is not directory, try to treat it as a file by reading its value
	if !fi.IsDir() {
		buf, err := os.ReadFile(path)
		if err == nil {
			return createTarredFileOf(string(buf), config)
		}
	}

	mode, uid, gid, err := makeTarFileEntryParams(config)
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("failed parsing target file parameters")
	}

	subdir := os.DirFS(path)
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)

	// build the tar by walking instead of using archive/tar.Writer.AddFS to be able to adjust mode, gid and uid
	err = fs.WalkDir(subdir, ".", func(filePath string, d fs.DirEntry, err error) error {
		header := &tar.Header{
			Name:    filepath.Join(config.Target, filePath),
			Mode:    mode,
			ModTime: time.Now(),
			Uid:     uid,
			Gid:     gid,
		}

		if d.IsDir() {
			// tar requires that directory headers ends with a slash
			header.Name = header.Name + "/"
			err = tarWriter.WriteHeader(header)
			if err != nil {
				return fmt.Errorf("failed writing tar header of directory %v while walking diretory structure, error was: %w", filePath, err)
			}
		} else {
			f, err := subdir.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()

			valueAsBytes, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("failed reading file %v for to send to container, error was: %w", filePath, err)
			}

			header.Size = int64(len(valueAsBytes))
			err = tarWriter.WriteHeader(header)
			if err != nil {
				return fmt.Errorf("failed writing tar header for file %v while walking diretory structure, error was: %w", filePath, err)
			}

			_, err = tarWriter.Write(valueAsBytes)
			if err != nil {
				return fmt.Errorf("failed writing file content of %v into tar archive while walking directory structure, error was: %w", filePath, err)
			}
		}

		return nil
	})

	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("failed building tar archive while walking config directory structure, error was: %w", err)
	}

	err = tarWriter.Close()
	if err != nil {
		return bytes.Buffer{}, fmt.Errorf("failed closing tar archive after writing, error was: %w", err)
	}

	return b, err
}

func makeTarFileEntryParams(config types.FileReferenceConfig) (mode int64, uid, gid int, err error) {
	mode = 0o444
	if config.Mode != nil {
		mode = int64(*config.Mode)
	}

	if config.UID != "" {
		v, err := strconv.Atoi(config.UID)
		if err != nil {
			return 0, 0, 0, err
		}
		uid = v
	}
	if config.GID != "" {
		v, err := strconv.Atoi(config.GID)
		if err != nil {
			return 0, 0, 0, err
		}
		gid = v
	}

	return mode, uid, gid, nil
}
