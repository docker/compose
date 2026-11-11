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
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/command"
	"github.com/opencontainers/go-digest"
	spec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/internal/oci"
	"github.com/docker/compose/v5/internal/registry"
	"github.com/docker/compose/v5/pkg/api"
)

const (
	OCI_REMOTE_ENABLED = "COMPOSE_EXPERIMENTAL_OCI_REMOTE"
	OciPrefix          = "oci://"
)

// validatePathInBase ensures a file path is contained within the base directory,
// as OCI artifact resources must all live within the same folder.
func validatePathInBase(base, unsafePath string) error {
	// Reject paths with path separators regardless of OS
	if strings.ContainsAny(unsafePath, "\\/") {
		return errors.New("invalid OCI artifact")
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
		return errors.New("invalid OCI artifact")
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
	return &ociRemoteLoader{
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

	// HTTP transport for the OCI resolver, initialized lazily so DD
	// detection happens once per loader rather than per Load() call.
	transportOnce sync.Once
	transport     http.RoundTripper
}

func (g *ociRemoteLoader) httpTransport(ctx context.Context) http.RoundTripper {
	g.transportOnce.Do(func() {
		g.transport = desktop.ProxyTransportFor(ctx, g.dockerCli.Client())
	})
	return g.transport
}

func (g *ociRemoteLoader) Accept(path string) bool {
	return strings.HasPrefix(path, OciPrefix)
}

func (g *ociRemoteLoader) Load(ctx context.Context, path string) (string, error) {
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
		local, err = g.pullComposeArtifact(ctx, path)
		if err != nil {
			return "", err
		}
		g.known[path] = local
	}
	return filepath.Join(local, "compose.yaml"), nil
}

// pullComposeArtifact resolves an oci:// path and pulls the compose artifact
// files into the local cache, unless already cached
func (g *ociRemoteLoader) pullComposeArtifact(ctx context.Context, path string) (string, error) {
	ref, err := reference.ParseDockerRef(path[len(OciPrefix):])
	if err != nil {
		return "", err
	}

	credentials := registry.NewDesktopAuthProvider(ctx, g.dockerCli.ConfigFile())
	resolver := oci.NewResolver(credentials, g.httpTransport(ctx), g.insecureRegistries...)

	descriptor, content, err := oci.Get(ctx, resolver, ref)
	if err != nil {
		return "", fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
	}

	cache, err := cacheDir()
	if err != nil {
		return "", fmt.Errorf("initializing remote resource cache: %w", err)
	}
	cache = filepath.Join(cache, cacheOCIDir)

	local := cacheEntryDir(cache, descriptor.Digest)
	if cacheEntryMatches(cache, descriptor.Digest) {
		return local, nil
	}

	// the manifest the digest is the one of, which the entry holds to tell it is of
	// that digest, and not of what has been put there otherwise
	root := content

	// a Compose application bundle is published as an image index
	isIndex, err := isIndexDocument(descriptor, content)
	if err != nil {
		return "", fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
	}
	if isIndex {
		content, err = g.resolveComposeManifest(ctx, resolver, ref, content)
		if err != nil {
			return "", err
		}
	}

	var manifest spec.Manifest
	err = json.Unmarshal(content, &manifest)
	if err != nil {
		return "", err
	}

	// the entry is extracted aside and moved in place once complete, so that what
	// is in the cache is never half written, by an interrupted or a concurrent run
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return "", fmt.Errorf("initializing remote resource cache: %w", err)
	}
	tmp, err := os.MkdirTemp(cache, ".pull-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // nothing left to remove once moved

	err = g.pullComposeFiles(ctx, tmp, manifest, ref, resolver)
	if err != nil {
		return "", err
	}
	// what is loaded from the entry: one that holds none is not complete, however
	// the manifest it was extracted from was served
	if info, err := os.Stat(filepath.Join(tmp, "compose.yaml")); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s holds no compose file", ref)
	}
	// last, so that nothing the artifact says can leave an entry without it
	if err := writeCacheEntryManifest(tmp, root); err != nil {
		return "", fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
	}
	if err := moveCacheEntry(cache, tmp, descriptor.Digest); err != nil {
		return "", err
	}
	return local, nil
}

// isIndexDocument tells whether the document the registry served for the
// reference is an image index, from what the document says it is. The media type
// the registry reports with it is not authenticated, the digest is the one of
// the content only: a registry that reports another one for authentic bytes is
// refused, rather than have them read as what they are not.
func isIndexDocument(descriptor spec.Descriptor, content []byte) (bool, error) {
	var document struct {
		MediaType string          `json:"mediaType"`
		Manifests json.RawMessage `json:"manifests"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return false, fmt.Errorf("invalid manifest %s: %w", descriptor.Digest, err)
	}
	isIndex := images.IsIndexType(document.MediaType) || (document.MediaType == "" && document.Manifests != nil)
	reported := descriptor.MediaType
	if (images.IsIndexType(reported) || images.IsManifestType(reported)) && images.IsIndexType(reported) != isIndex {
		return false, fmt.Errorf("manifest %s is reported as %s, which it is not", descriptor.Digest, reported)
	}
	return isIndex, nil
}

// cacheOCIDir is the directory of the cache which holds the entries of the OCI
// artifacts, one directory of the extracted project for each digest. Earlier
// releases cached them at the root of the cache, where they are no longer looked
// for: nothing which one of them left, and which an artifact could have filled
// with what it chose, is taken for an entry.
const cacheOCIDir = "oci"

// cacheEntryManifestDir is the directory of an entry which holds its manifest, in
// the file cacheEntryManifestFile: the one the entry was extracted from, as the
// registry served it, which is the index when the reference is an image index.
// An artifact writes the files of an entry by names which have no path separator
// (see validatePathInBase), so it can't write inside a directory of it: what is
// there was put by Compose, and the entry is not taken for one without it.
const (
	cacheEntryManifestDir  = ".manifest"
	cacheEntryManifestFile = "manifest.json"
)

// cacheEntryDir is the directory of the entry of the cache for the digest.
func cacheEntryDir(cache string, dgst digest.Digest) string {
	return filepath.Join(cache, dgst.Hex())
}

// writeCacheEntryManifest puts the manifest in the directory extracted in dir,
// which fails when the artifact has a file of the name of the directory that
// holds it.
func writeCacheEntryManifest(dir string, manifest []byte) error {
	holder := filepath.Join(dir, cacheEntryManifestDir)
	if err := os.Mkdir(holder, 0o700); err != nil {
		return fmt.Errorf("invalid OCI artifact: %w", err)
	}
	return os.WriteFile(filepath.Join(holder, cacheEntryManifestFile), manifest, 0o600)
}

// cacheEntryMatches tells whether the cache holds a complete entry for the digest:
// a directory named after it which holds the manifest that has it. A directory
// without one is not an entry, which is what releases that did not verify what
// they fetched left, or an extraction that was interrupted, and neither is one
// holding the manifest of another digest.
func cacheEntryMatches(cache string, dgst digest.Digest) bool {
	if dgst.Validate() != nil {
		return false
	}
	manifest, err := os.ReadFile(filepath.Join(cacheEntryDir(cache, dgst), cacheEntryManifestDir, cacheEntryManifestFile))
	if err != nil {
		return false
	}
	return dgst.Algorithm().FromBytes(manifest) == dgst
}

// moveCacheEntry puts the entry extracted in tmp in place, unless another run
// did already, and replaces what is there when it is not an entry of the cache
// for the digest. The entry holds its manifest, so that it is complete as soon as
// it is in place.
//
// What is replaced is first set aside, in a directory of its own, which takes
// it out of the cache in one step, and only then removed: a run that replaces
// it at the same time does not remove the entry that another one has put in
// place in the meantime, and the cache is left with a complete entry whichever
// of them is last. What was set aside is not put back when the replacement
// fails, whatever it is: another run may have set aside the entry that took its
// place, and would then find it in its way, and the worst that follows is that
// the artifact is extracted again. A run that loses to another may still return
// an error: what it gets is the state of the cache when it is over, not the
// result of its own move.
func moveCacheEntry(cache, tmp string, dgst digest.Digest) error {
	local := cacheEntryDir(cache, dgst)
	err := os.Rename(tmp, local)
	if err == nil || cacheEntryMatches(cache, dgst) {
		return nil
	}

	aside, err := os.MkdirTemp(cache, ".stale-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(aside) }()
	if err := os.Rename(local, filepath.Join(aside, filepath.Base(local))); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmp, local); err != nil {
		if cacheEntryMatches(cache, dgst) {
			return nil
		}
		return err
	}
	return nil
}

// resolveComposeManifest returns the content of the compose artifact manifest
// referenced by an image index
func (g *ociRemoteLoader) resolveComposeManifest(ctx context.Context, resolver remotes.Resolver, ref reference.Named, content []byte) ([]byte, error) {
	var index spec.Index
	err := json.Unmarshal(content, &index)
	if err != nil {
		return nil, err
	}
	found := false
	for _, manifest := range index.Manifests {
		if manifest.ArtifactType != oci.ComposeProjectArtifactType {
			continue
		}
		found = true
		digested, err := reference.WithDigest(ref, manifest.Digest)
		if err != nil {
			return nil, err
		}
		_, content, err = oci.Get(ctx, resolver, digested)
		if err != nil {
			return nil, fmt.Errorf("failed to pull OCI resource %q: %w", ref, err)
		}
	}
	if !found {
		return nil, fmt.Errorf("OCI index %s doesn't refer to compose artifacts", ref)
	}
	return content, nil
}

func (g *ociRemoteLoader) Dir(path string) string {
	return g.known[path]
}

func (g *ociRemoteLoader) pullComposeFiles(ctx context.Context, local string, manifest spec.Manifest, ref reference.Named, resolver remotes.Resolver) error {
	err := os.MkdirAll(local, 0o700)
	if err != nil {
		return err
	}
	if (manifest.ArtifactType != "" && manifest.ArtifactType != oci.ComposeProjectArtifactType) ||
		(manifest.ArtifactType == "" && manifest.Config.MediaType != oci.ComposeEmptyConfigMediaType) {
		return fmt.Errorf("%s is not a compose project OCI artifact, but %s", ref.String(), manifest.ArtifactType)
	}

	for i, layer := range manifest.Layers {
		content, err := oci.GetBlob(ctx, resolver, ref, layer)
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

var _ loader.ResourceLoader = (*ociRemoteLoader)(nil)
