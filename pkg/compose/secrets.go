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
	"slices"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
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

		if mount.Target == "" {
			if mountType == secretMount {
				mount.Target = "/run/secrets/" + mount.Source
			} else {
				mount.Target = "/" + mount.Source
			}
		} else if mountType == secretMount && !isAbsTarget(mount.Target) {
			mount.Target = "/run/secrets/" + mount.Target
		}

		if err := s.copyFileToContainer(ctx, id, content, mount); err != nil {
			return err
		}
	}
	return nil
}

// warnIgnoredFileReferences warns about uid/gid/mode set on a service-level
// configs:/secrets: reference that this runtime will not apply. Whether they
// are applied is a property of how the referenced object reaches the
// container, which is only settled here, at creation time: content and
// environment sources are copied in (injectFileReferences) and honor the
// overrides, while a `file:` source is bind-mounted as-is and does not.
// Deciding it when the compose file is loaded would hard-wire that answer
// into the model, whereas the engine may later be able to honor ownership on
// bind mounts too; fileReferenceOverridesIgnored is the single place to teach
// about such a capability.
func warnIgnoredFileReferences(project *types.Project, services []string) {
	for _, name := range slices.Sorted(slices.Values(services)) {
		service, ok := project.Services[name]
		if !ok {
			continue
		}
		for _, ref := range service.Configs {
			warnIgnoredFileReference(name, "configs", types.FileReferenceConfig(ref), types.FileObjectConfig(project.Configs[ref.Source]))
		}
		for _, ref := range service.Secrets {
			warnIgnoredFileReference(name, "secrets", types.FileReferenceConfig(ref), types.FileObjectConfig(project.Secrets[ref.Source]))
		}
	}
}

func warnIgnoredFileReference(service, kind string, ref types.FileReferenceConfig, object types.FileObjectConfig) {
	if !fileReferenceOverridesIgnored(object) {
		return
	}
	source := ref.Source
	if source == "" {
		source = "(anonymous)"
	}
	for _, o := range []struct {
		field string
		set   bool
	}{{"uid", ref.UID != ""}, {"gid", ref.GID != ""}, {"mode", ref.Mode != nil}} {
		if !o.set {
			continue
		}
		logrus.Warn(api.UnsupportedAttribute{
			Service: service,
			Path:    fmt.Sprintf("%s.%s.%s", kind, source, o.field),
			Reason:  o.field + " is not supported outside Swarm mode and will be ignored",
		})
	}
}

// fileReferenceOverridesIgnored reports whether uid/gid/mode on a reference to
// object are dropped: they are for an object that is bind-mounted, which is
// what buildContainerConfigMounts and buildContainerSecretMounts do with
// whatever injectFileReferences does not copy in (content and environment).
// External and driver-backed objects are rejected when mounts are built, so
// they get that error rather than a warning.
func fileReferenceOverridesIgnored(object types.FileObjectConfig) bool {
	if object.External || object.Driver != "" || object.TemplateDriver != "" {
		return false
	}
	return object.Content == "" && object.Environment == ""
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

func (s *composeService) copyFileToContainer(ctx context.Context, id, content string, file types.FileReferenceConfig) error {
	b, err := createTar(content, file)
	if err != nil {
		return err
	}

	_, err = s.apiClient().CopyToContainer(ctx, id, client.CopyToContainerOptions{
		DestinationPath: "/",
		Content:         &b,
		CopyUIDGID:      file.UID != "" || file.GID != "",
	})
	return err
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
