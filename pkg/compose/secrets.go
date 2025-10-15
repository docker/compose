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
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
)

type mountType string

const (
	secretMount mountType = "secret"
	configMount mountType = "config"
)

func (s *composeService) injectSecrets(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	return s.injectFileReferences(ctx, project, service, id, secretMount)
}

func (s *composeService) injectConfigs(ctx context.Context, project *types.Project, service types.ServiceConfig, id string) error {
	return s.injectFileReferences(ctx, project, service, id, configMount)
}

func (s *composeService) injectFileReferences(ctx context.Context, project *types.Project, service types.ServiceConfig, id string, mountType mountType) error {
	mounts, sources := s.getFilesAndMap(project, service, mountType)
	var ctrConfig *container.Config

	for _, mount := range mounts {
		content, err := s.resolveFileContent(project, sources[mount.Source], mountType)
		if err != nil {
			return err
		}
		if content == "" {
			continue
		}

		if service.ReadOnly {
			return fmt.Errorf("cannot create %s %q in read-only service %s: `file` is the sole supported option", mountType, sources[mount.Source].Name, service.Name)
		}

		s.setDefaultTarget(&mount, mountType)

		ctrConfig, err = s.setFileOwnership(ctx, id, &mount, ctrConfig)
		if err != nil {
			return err
		}

		if err := s.copyFileToContainer(ctx, id, content, mount); err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) getFilesAndMap(project *types.Project, service types.ServiceConfig, mountType mountType) ([]types.FileReferenceConfig, map[string]types.FileObjectConfig) {
	var files []types.FileReferenceConfig
	var fileMap map[string]types.FileObjectConfig

	switch mountType {
	case secretMount:
		files = make([]types.FileReferenceConfig, len(service.Secrets))
		for i, config := range service.Secrets {
			files[i] = types.FileReferenceConfig(config)
		}
		fileMap = make(map[string]types.FileObjectConfig)
		for k, v := range project.Secrets {
			fileMap[k] = types.FileObjectConfig(v)
		}
	case configMount:
		files = make([]types.FileReferenceConfig, len(service.Configs))
		for i, config := range service.Configs {
			files[i] = types.FileReferenceConfig(config)
		}
		fileMap = make(map[string]types.FileObjectConfig)
		for k, v := range project.Configs {
			fileMap[k] = types.FileObjectConfig(v)
		}
	}
	return files, fileMap
}

func (s *composeService) resolveFileContent(project *types.Project, source types.FileObjectConfig, mountType mountType) (string, error) {
	if source.Content != "" {
		// inlined, or already resolved by include
		return source.Content, nil
	}
	if source.Environment != "" {
		env, ok := project.Environment[source.Environment]
		if !ok {
			return "", fmt.Errorf("environment variable %q required by %s %q is not set", source.Environment, mountType, source.Name)
		}
		return env, nil
	}
	return "", nil
}

func (s *composeService) setDefaultTarget(file *types.FileReferenceConfig, mountType mountType) {
	if file.Target == "" {
		if mountType == secretMount {
			file.Target = "/run/secrets/" + file.Source
		} else {
			file.Target = "/" + file.Source
		}
	} else if mountType == secretMount && !isAbsTarget(file.Target) {
		file.Target = "/run/secrets/" + file.Target
	}
}

func (s *composeService) setFileOwnership(ctx context.Context, id string, file *types.FileReferenceConfig, ctrConfig *container.Config) (*container.Config, error) {
	if file.UID != "" || file.GID != "" {
		return ctrConfig, nil
	}

	if ctrConfig == nil {
		ctr, err := s.apiClient().ContainerInspect(ctx, id)
		if err != nil {
			return nil, err
		}
		ctrConfig = ctr.Config
	}

	parts := strings.Split(ctrConfig.User, ":")
	if len(parts) > 0 {
		file.UID = parts[0]
	}
	if len(parts) > 1 {
		file.GID = parts[1]
	}

	return ctrConfig, nil
}

func (s *composeService) copyFileToContainer(ctx context.Context, id, content string, file types.FileReferenceConfig) error {
	b, err := createTar(content, file)
	if err != nil {
		return err
	}

	return s.apiClient().CopyToContainer(ctx, id, "/", &b, container.CopyToContainerOptions{
		CopyUIDGID: true,
	})
}

func createTar(env string, config types.FileReferenceConfig) (bytes.Buffer, error) {
	value := []byte(env)
	b := bytes.Buffer{}
	tarWriter := tar.NewWriter(&b)
	mode := types.FileMode(0o444)
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
