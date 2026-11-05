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
	"sync"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/oci"
)

// cacheTestRegistry serves a compose project artifact of a single compose file
// under any reference of test/artifact.
type cacheTestRegistry struct {
	manifest []byte
	digest   digest.Digest
	compose  []byte
	// what the blob endpoint serves, which is the compose file unless a test says otherwise
	blob []byte
}

func newCacheTestRegistry(t *testing.T) *cacheTestRegistry {
	t.Helper()
	compose := []byte("services:\n  app:\n    image: alpine\n")
	manifest, err := json.Marshal(spec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: oci.ComposeProjectArtifactType,
		Config:       spec.Descriptor{MediaType: oci.ComposeEmptyConfigMediaType, Digest: digest.FromString("{}"), Size: 2},
		Layers:       []spec.Descriptor{{MediaType: oci.ComposeYAMLMediaType, Digest: digest.FromBytes(compose), Size: int64(len(compose))}},
	})
	assert.NilError(t, err)
	return &cacheTestRegistry{manifest: manifest, digest: digest.FromBytes(manifest), compose: compose, blob: compose}
}

// serve starts the registry and returns its host, which the loader of the test must be told is insecure.
func (r *cacheTestRegistry) serve(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/v2/":
		case strings.HasPrefix(req.URL.Path, "/v2/test/artifact/manifests/"):
			w.Header().Set("Content-Type", spec.MediaTypeImageManifest)
			w.Header().Set("Docker-Content-Digest", r.digest.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(r.manifest))) // also for HEAD, which resolves
			if req.Method != http.MethodHead {
				_, _ = w.Write(r.manifest)
			}
		case req.URL.Path == "/v2/test/artifact/blobs/"+digest.FromBytes(r.compose).String():
			_, _ = w.Write(r.blob)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

func (r *cacheTestRegistry) ref(host string) string {
	return OciPrefix + host + "/test/artifact@" + r.digest.String()
}

func newCacheTestLoader(host string) *ociRemoteLoader {
	l := &ociRemoteLoader{known: map[string]string{}, insecureRegistries: []string{host}}
	l.transportOnce.Do(func() { l.transport = &http.Transport{} })
	l.credentialsOnce.Do(func() { l.credentials = &configfile.ConfigFile{} })
	return l
}

// setCacheRoot points the cache of extracted projects to a new directory, which it returns.
func setCacheRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	return filepath.Join(root, "docker-compose")
}

// An entry of the cache of extracted projects is complete or is not there: it
// is extracted aside and moved in place in one step, so that a run never uses
// what an interrupted or a concurrent run is still writing, and it is marked
// as complete, so that a directory which is not one is not taken for an entry.
func TestOCILoaderCacheEntryIsComplete(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)
	cache := setCacheRoot(t)

	file, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
	assert.NilError(t, err)
	assert.Equal(t, file, filepath.Join(cache, registry.digest.Hex(), "compose.yaml"))
	content, err := os.ReadFile(file)
	assert.NilError(t, err)
	assert.Equal(t, string(content), string(registry.compose))

	entries, err := os.ReadDir(cache)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1, "no directory is left aside")
	_, err = os.Stat(filepath.Join(cache, registry.digest.Hex(), cacheEntryComplete))
	assert.NilError(t, err)

	t.Run("a complete entry is used as it is", func(t *testing.T) {
		kept := filepath.Join(cache, registry.digest.Hex(), "kept")
		assert.NilError(t, os.WriteFile(kept, nil, 0o600))
		_, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
		assert.NilError(t, err)
		_, err = os.Stat(kept)
		assert.NilError(t, err)
	})
}

// A directory of the cache which is not marked as complete is extracted again,
// whatever it holds: an interrupted extraction leaves some of the files, and an
// entry written by a release which did not mark them looks like a complete one.
func TestOCILoaderExtractsAgainWhatIsNotACompleteEntry(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)

	for name, compose := range map[string]string{
		"an interrupted extraction":                    "services:\n  app:\n    ima",
		"an entry of a release that did not mark them": string(registry.compose),
	} {
		t.Run(name, func(t *testing.T) {
			cache := setCacheRoot(t)
			entry := filepath.Join(cache, registry.digest.Hex())
			assert.NilError(t, os.MkdirAll(entry, 0o700))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "compose.yaml"), []byte(compose), 0o600))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "stale"), nil, 0o600))

			file, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
			assert.NilError(t, err)
			assert.Equal(t, file, filepath.Join(entry, "compose.yaml"))
			content, err := os.ReadFile(file)
			assert.NilError(t, err)
			assert.Equal(t, string(content), string(registry.compose))
			_, err = os.Stat(filepath.Join(entry, "stale"))
			assert.Assert(t, os.IsNotExist(err), "the directory is replaced, not completed")
			_, err = os.Stat(filepath.Join(entry, cacheEntryComplete))
			assert.NilError(t, err)
		})
	}
}

// A pull that fails leaves nothing in the cache, neither an entry nor what
// it was extracting.
func TestOCILoaderFailedPullLeavesNothingBehind(t *testing.T) {
	registry := newCacheTestRegistry(t)
	registry.blob = []byte("services:\n  app:\n    image: alp1ne\n") // as long as the layer, not the same
	host := registry.serve(t)
	cache := setCacheRoot(t)

	_, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
	assert.ErrorContains(t, err, "digest mismatch")

	entries, err := os.ReadDir(cache)
	assert.Assert(t, err == nil || os.IsNotExist(err))
	assert.Equal(t, len(entries), 0)
}

// Runs that load the same artifact at the same time, whether the cache has the
// entry already, has none or has a directory that is not one, all succeed and
// leave a single complete entry, and nothing aside. What a run gets is the path
// of the entry, which is read once the runs are over: a run that replaces a
// directory which is not an entry takes it out of the way for a moment.
func TestOCILoaderConcurrentRunsLeaveOneCompleteEntry(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)

	for name, seed := range map[string]func(t *testing.T, entry string){
		"no entry": func(*testing.T, string) {},
		"a directory that is not an entry": func(t *testing.T, entry string) {
			assert.NilError(t, os.MkdirAll(entry, 0o700))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "compose.yaml"), []byte("services:\n  app:\n    ima"), 0o600))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "stale"), nil, 0o600))
		},
	} {
		t.Run(name, func(t *testing.T) {
			cache := setCacheRoot(t)
			entry := filepath.Join(cache, registry.digest.Hex())
			seed(t, entry)

			const runs = 8
			files := make([]string, runs)
			errs := make([]error, runs)
			var wg sync.WaitGroup
			for i := range runs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					files[i], errs[i] = newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
				}()
			}
			wg.Wait()

			for i := range runs {
				assert.NilError(t, errs[i])
				assert.Equal(t, files[i], filepath.Join(entry, "compose.yaml"))
			}
			content, err := os.ReadFile(filepath.Join(entry, "compose.yaml"))
			assert.NilError(t, err)
			assert.Equal(t, string(content), string(registry.compose))
			assert.Assert(t, cacheEntryIsComplete(entry))
			_, err = os.Stat(filepath.Join(entry, "stale"))
			assert.Assert(t, os.IsNotExist(err))
			entries, err := os.ReadDir(cache)
			assert.NilError(t, err)
			assert.Equal(t, len(entries), 1, "no directory is left aside")
		})
	}
}
