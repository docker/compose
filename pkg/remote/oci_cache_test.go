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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/oci"
)

// composeArtifact is a registry serving a compose project artifact of a single compose file, under any
// reference of test/artifact, and the digest it claims for the manifest.
type composeArtifact struct {
	manifest []byte
	claimed  digest.Digest
	layer    []byte
	// the digest of the layer in the manifest, which the blob is served under
	layerDigest digest.Digest
}

func newComposeArtifact(t *testing.T, composeFile string) composeArtifact {
	t.Helper()
	layer := []byte(composeFile)
	manifest, err := json.Marshal(spec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: oci.ComposeProjectArtifactType,
		Config:       spec.Descriptor{MediaType: oci.ComposeEmptyConfigMediaType, Digest: digest.FromString("{}"), Size: 2},
		Layers:       []spec.Descriptor{{MediaType: oci.ComposeYAMLMediaType, Digest: digest.FromBytes(layer), Size: int64(len(layer))}},
	})
	assert.NilError(t, err)
	return composeArtifact{manifest: manifest, claimed: digest.FromBytes(manifest), layer: layer, layerDigest: digest.FromBytes(layer)}
}

func (a composeArtifact) serve(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
		case strings.HasPrefix(r.URL.Path, "/v2/test/artifact/manifests/"):
			w.Header().Set("Content-Type", spec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", a.claimed.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(a.manifest))) // also for HEAD, which resolves
			if r.Method != http.MethodHead {
				_, _ = w.Write(a.manifest)
			}
		case r.URL.Path == "/v2/test/artifact/blobs/"+a.layerDigest.String():
			_, _ = w.Write(a.layer)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// A registry can't make Compose cache its content as the one of another digest: what it serves is
// verified against the digest it is requested or resolved by, so that loading a pinned digest later, from
// the registry that holds it, doesn't give the content of the registry loaded before.
func TestOCILoaderCacheIsNotPoisonedByAnotherRegistry(t *testing.T) {
	trusted := newComposeArtifact(t, "services:\n  app:\n    image: alpine\n")
	rogue := newComposeArtifact(t, "services:\n  app:\n    image: evil/image\n")
	rogue.claimed = trusted.claimed // says it holds the manifest that the trusted registry holds

	trustedHost := trusted.serve(t)
	rogueHost := rogue.serve(t)

	for name, ref := range map[string]string{
		"tag":    "oci://" + rogueHost + "/test/artifact:latest",
		"digest": "oci://" + rogueHost + "/test/artifact@" + trusted.claimed.String(),
	} {
		t.Run(name, func(t *testing.T) {
			cache := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", cache)

			// the rogue registry's content is refused...
			_, err := newCacheTestLoader(rogueHost).Load(t.Context(), ref)
			assert.ErrorContains(t, err, "digest mismatch")
			entries, err := os.ReadDir(filepath.Join(cache, "docker-compose", cacheOCIDir))
			assert.Assert(t, err == nil || os.IsNotExist(err))
			assert.Equal(t, len(entries), 0, "nothing is cached by the rejected load")

			// ...so the pinned digest gives the trusted registry's
			file, err := newCacheTestLoader(trustedHost).Load(t.Context(), "oci://"+trustedHost+"/test/artifact@"+trusted.claimed.String())
			assert.NilError(t, err)
			content, err := os.ReadFile(file)
			assert.NilError(t, err)
			assert.Equal(t, string(content), string(trusted.layer))
		})
	}
}
