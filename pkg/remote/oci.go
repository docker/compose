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

package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v5/internal/oci"
	"github.com/docker/compose/v5/pkg/api"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	OCI_REMOTE_ENABLED = "COMPOSE_EXPERIMENTAL_OCI_REMOTE"
	OciPrefix          = "oci://"
)

// validatePathInBase ensures a file path is contained within the base directory,
// as OCI artifacts resources must all live within the same folder.
func validatePathInBase(base, unsafePath string) error {
	// Reject paths with path separators regardless of OS
	if strings.ContainsAny(unsafePath, "\\/") {
		return fmt.Errorf("invalid OCI artifact")
	}

	// Join the base with the untrusted path
	targetPath := filepath.Join(base, unsafePath)

	// Get the directory of the target path
	targetDir := filepath.Dir(targetPath)

	// Clean both paths to resolve any .. or . components
	cleanBase := filepath.Clean(base)
	cleanTargetDir := filepath.Clean(targetDir)

	// Check if the target directory is the same as base directory
	if cleanTargetDir != cleanBase {
		return fmt.Errorf("invalid OCI artifact")
	}

	return nil
}

func ociRemoteLoaderEnabled() (bool, error) {
	if v := os.Getenv(OCI_REMOTE_ENABLED); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("COMPOSE_EXPERIMENTAL_OCI_REMOTE environment variable expects boolean value: %w", err)
		}
		return enabled, err
	}
	return true, nil
}

func NewOCIRemoteLoader(dockerCli command.Cli, offline bool, options api.OCIOptions) loader.ResourceLoader {
	return ociRemoteLoader{
		dockerCli:          dockerCli,
		offline:            offline,
		known:              map[string]string{},
		insecureRegistries: options.InsecureRegistries,
	}
}

type ociRemoteLoader struct {
	dockerCli          command.Cli
	offline            bool
	known              map[string]string
	insecureRegistries []string
}

func (g ociRemoteLoader) Accept(path string) bool {
	return strings.HasPrefix(path, OciPrefix)
}

//nolint:gocyclo
func (g ociRemoteLoader) Load(ctx context.Context, path string) (string, error) {
	enabled, err := ociRemoteLoaderEnabled()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("OCI remote resource is disabled by %q", OCI_REMOTE_ENABLED)
	}

	if g.offline {
		return "", nil
	}

	local, ok := g.known[path]
	if !ok {
		ref, err := reference.ParseDockerRef(path[len(OciPrefix):])
		if err != nil {
			return "", err
		}

		resolver := oci.NewResolver(g.dockerCli.ConfigFile(), g.insecureRegistries...)

		descriptor, content, err := oci.Get(ctx, resolver, ref)
		if err != nil {
			return "", fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
		}

		cache, err := cacheDir()
		if err != nil {
			return "", fmt.Errorf("initializing remote resource cache: %w", err)
		}

		local = filepath.Join(cache, descriptor.Digest.Hex())
		if _, err = os.Stat(local); os.IsNotExist(err) {

			// a Compose application bundle is published as image index
			if images.IsIndexType(descriptor.MediaType) {
				var index spec.Index
				err = json.Unmarshal(content, &index)
				if err != nil {
					return "", err
				}
				found := false
				for _, manifest := range index.Manifests {
					if manifest.ArtifactType != oci.ComposeProjectArtifactType {
						continue
					}
					found = true
					digested, err := reference.WithDigest(ref, manifest.Digest)
					if err != nil {
						return "", err
					}
					descriptor, content, err = oci.Get(ctx, resolver, digested)
					if err != nil {
						return "", fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
					}
				}
				if !found {
					return "", fmt.Errorf("OCI index %s doesn't refer to compose artifacts", ref)
				}
			}

			var manifest spec.Manifest
			err = json.Unmarshal(content, &manifest)
			if err != nil {
				return "", err
			}

			err = g.pullComposeFiles(ctx, local, manifest, ref, resolver)
			if err != nil {
				// we need to clean up the directory to be sure we won't let empty files present
				_ = os.RemoveAll(local)
				return "", err
			}
		}
		g.known[path] = local
	}
	return filepath.Join(local, "compose.yaml"), nil
}

func (g ociRemoteLoader) Dir(path string) string {
	return g.known[path]
}

func (g ociRemoteLoader) pullComposeFiles(ctx context.Context, local string, manifest spec.Manifest, ref reference.Named, resolver remotes.Resolver) error {
	err := os.MkdirAll(local, 0o700)
	if err != nil {
		return err
	}
	if (manifest.ArtifactType != "" && manifest.ArtifactType != oci.ComposeProjectArtifactType) ||
		(manifest.ArtifactType == "" && manifest.Config.MediaType != oci.ComposeEmptyConfigMediaType) {
		return fmt.Errorf("%s is not a compose project OCI artifact, but %s", ref.String(), manifest.ArtifactType)
	}

	for i, layer := range manifest.Layers {
		digested, err := reference.WithDigest(ref, layer.Digest)
		if err != nil {
			return err
		}

		_, content, err := oci.Get(ctx, resolver, digested)
		if err != nil {
			return err
		}

		switch layer.MediaType {
		case oci.ComposeYAMLMediaType:
			if err := writeComposeFile(layer, i, local, content); err != nil {
				return err
			}
		case oci.ComposeEnvFileMediaType:
			if err := writeEnvFile(layer, local, content); err != nil {
				return err
			}
		case oci.ComposeEmptyConfigMediaType:
		}
	}
	return nil
}

func writeComposeFile(layer spec.Descriptor, i int, local string, content []byte) error {
	file := "compose.yaml"
	if _, ok := layer.Annotations["com.docker.compose.extends"]; ok {
		file = layer.Annotations["com.docker.compose.file"]
		if err := validatePathInBase(local, file); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(filepath.Join(local, file), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, ok := layer.Annotations["com.docker.compose.file"]; i > 0 && ok {
		_, err := f.Write([]byte("\n---\n"))
		if err != nil {
			return err
		}
	}
	_, err = f.Write(content)
	return err
}

func writeEnvFile(layer spec.Descriptor, local string, content []byte) error {
	envfilePath, ok := layer.Annotations["com.docker.compose.envfile"]
	if !ok {
		return fmt.Errorf("missing annotation com.docker.compose.envfile in layer %q", layer.Digest)
	}
	if err := validatePathInBase(local, envfilePath); err != nil {
		return err
	}
	otherFile, err := os.Create(filepath.Join(local, envfilePath))
	if err != nil {
		return err
	}
	defer func() { _ = otherFile.Close() }()
	_, err = otherFile.Write(content)
	return err
}

var _ loader.ResourceLoader = ociRemoteLoader{}
