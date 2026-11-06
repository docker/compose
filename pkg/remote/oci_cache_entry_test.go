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
	"cmp"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
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
	// the media type reported with the manifest, which is the one of a manifest unless a test says otherwise
	contentType string
	// what is served for a reference, which is a digest, instead of the manifest: a rogue registry tells
	// it is the content of the digest
	byReference map[digest.Digest][]byte
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
			manifest, claimed := r.manifest, r.digest
			if served, ok := r.byReference[digest.Digest(path.Base(req.URL.Path))]; ok {
				manifest, claimed = served, digest.Digest(path.Base(req.URL.Path))
			}
			w.Header().Set("Content-Type", cmp.Or(r.contentType, spec.MediaTypeImageManifest))
			w.Header().Set("Docker-Content-Digest", claimed.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(manifest))) // also for HEAD, which resolves
			if req.Method != http.MethodHead {
				_, _ = w.Write(manifest)
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

// setCacheRoot points the cache to a new directory, and returns the one of the OCI artifacts in it.
func setCacheRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	return filepath.Join(root, "docker-compose", cacheOCIDir)
}

// An entry of the cache of extracted projects is complete or is not there: it
// is extracted aside, with its manifest, and moved in place in one step, so that
// a run never uses what an interrupted or a concurrent run is still writing, and
// a directory which holds no manifest is not taken for an entry.
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
	assert.DeepEqual(t, entryNames(entries), []string{registry.digest.Hex()}) // no directory is left aside
	manifest, err := os.ReadFile(filepath.Join(cache, registry.digest.Hex(), cacheEntryManifestDir, cacheEntryManifestFile))
	assert.NilError(t, err)
	assert.Equal(t, string(manifest), string(registry.manifest), "the entry holds the manifest it is named after")

	t.Run("a complete entry is used as it is", func(t *testing.T) {
		kept := filepath.Join(cache, registry.digest.Hex(), "kept")
		assert.NilError(t, os.WriteFile(kept, nil, 0o600))
		_, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
		assert.NilError(t, err)
		_, err = os.Stat(kept)
		assert.NilError(t, err)
	})
}

// A directory of the cache which holds no manifest of its digest is extracted
// again, whatever else it holds: an interrupted extraction leaves some of the
// files, an entry written by a release which did not verify what it fetched looks
// like a complete one, and so does a directory holding another manifest.
func TestOCILoaderExtractsAgainWhatIsNotACompleteEntry(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)

	evil := "services:\n  app:\n    image: evil/image\n"
	for name, tt := range map[string]struct {
		compose  string
		manifest *string // what the directory holds as its manifest, if anything
	}{
		"an interrupted extraction":                                 {compose: "services:\n  app:\n    ima"},
		"an entry of a release that did not verify what it fetched": {compose: evil},
		"a directory holding the manifest of another digest":        {compose: evil, manifest: ptr(`{"schemaVersion":2}`)},
	} {
		t.Run(name, func(t *testing.T) {
			cache := setCacheRoot(t)
			entry := filepath.Join(cache, registry.digest.Hex())
			assert.NilError(t, os.MkdirAll(filepath.Join(entry, cacheEntryManifestDir), 0o700))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "compose.yaml"), []byte(tt.compose), 0o600))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "stale"), nil, 0o600))
			if tt.manifest != nil {
				assert.NilError(t, os.WriteFile(filepath.Join(entry, cacheEntryManifestDir, cacheEntryManifestFile), []byte(*tt.manifest), 0o600))
			}

			file, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
			assert.NilError(t, err)
			assert.Equal(t, file, filepath.Join(entry, "compose.yaml"))
			content, err := os.ReadFile(file)
			assert.NilError(t, err)
			assert.Equal(t, string(content), string(registry.compose))
			_, err = os.Stat(filepath.Join(entry, "stale"))
			assert.Assert(t, os.IsNotExist(err), "the directory is replaced, not completed")
			assert.Assert(t, cacheEntryMatches(cache, registry.digest))
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

// Runs that load the same artifact at the same time, whether the cache has none
// or has a directory that is not an entry, leave a single complete entry, and
// nothing aside. When the cache has none, which is what a cold cache is, the
// path every run gets is the one of a file that is there as it gets it: the entry
// is in place complete or it is not there. Replacing a directory which is not an
// entry takes two steps, which a run that loses to another may see in between,
// and may fail on: what is pinned then is the path of the entry when the run
// succeeds, and the state of the cache once the runs are over.
func TestOCILoaderConcurrentRunsLeaveOneCompleteEntry(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)

	for name, tt := range map[string]struct {
		seed        func(t *testing.T, entry string)
		alwaysThere bool // the file of the path a run gets is there as it gets it
	}{
		"no entry": {seed: func(*testing.T, string) {}, alwaysThere: true},
		"a directory that is not an entry": {seed: func(t *testing.T, entry string) {
			assert.NilError(t, os.MkdirAll(entry, 0o700))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "compose.yaml"), []byte("services:\n  app:\n    ima"), 0o600))
			assert.NilError(t, os.WriteFile(filepath.Join(entry, "stale"), nil, 0o600))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			cache := setCacheRoot(t)
			entry := filepath.Join(cache, registry.digest.Hex())
			tt.seed(t, entry)

			const runs = 8
			files := make([]string, runs)
			errs := make([]error, runs)
			there := make([]bool, runs) // whether the file a run got is there when it gets it
			var wg sync.WaitGroup
			for i := range runs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					files[i], errs[i] = newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
					there[i] = errs[i] != nil || fileExists(files[i])
				}()
			}
			wg.Wait()

			for i := range runs {
				if errs[i] == nil {
					assert.Equal(t, files[i], filepath.Join(entry, "compose.yaml"))
					if tt.alwaysThere {
						assert.Assert(t, there[i], "run %d got a path to a file which was not there", i)
					}
				}
			}
			content, err := os.ReadFile(filepath.Join(entry, "compose.yaml"))
			assert.NilError(t, err)
			assert.Equal(t, string(content), string(registry.compose))
			assert.Assert(t, cacheEntryMatches(cache, registry.digest))
			_, err = os.Stat(filepath.Join(entry, "stale"))
			assert.Assert(t, os.IsNotExist(err))
			entries, err := os.ReadDir(cache)
			assert.NilError(t, err)
			assert.DeepEqual(t, entryNames(entries), []string{registry.digest.Hex()}) // no directory is left aside
		})
	}
}

// What is replaced and whose replacement then fails is not put back, whatever it
// is, and nothing of it is left aside: it would be in the way of the run that set
// aside the entry which took its place (see moveCacheEntry).
func TestMoveCacheEntryLeavesNothingAsideWhenItFails(t *testing.T) {
	cache := t.TempDir()
	dgst := digest.FromString("entry")
	local := filepath.Join(cache, dgst.Hex())
	assert.NilError(t, os.MkdirAll(local, 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(local, "compose.yaml"), []byte("services:\n  app:\n    ima"), 0o600))

	err := moveCacheEntry(cache, filepath.Join(cache, "missing"), dgst)
	assert.Assert(t, err != nil)

	entries, err := os.ReadDir(cache)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0, "neither the directory nor anything aside is left")
}

// The extracted artifacts are kept in a directory of the cache for OCI artifacts.
// What an earlier release cached is at the root of the cache, where nothing looks
// for it any more, whatever it holds, even what looks like a complete entry: a
// release which did not verify what it fetched may have cached what a registry
// chose, and the artifact is extracted again in the directory.
func TestOCILoaderKeepsEntriesInTheDirectoryForOCI(t *testing.T) {
	registry := newCacheTestRegistry(t)
	host := registry.serve(t)
	cache := setCacheRoot(t)
	legacy := filepath.Join(filepath.Dir(cache), registry.digest.Hex())
	assert.NilError(t, os.MkdirAll(filepath.Join(legacy, cacheEntryManifestDir), 0o700))
	evil := []byte("services:\n  app:\n    image: evil/image\n")
	assert.NilError(t, os.WriteFile(filepath.Join(legacy, "compose.yaml"), evil, 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(legacy, cacheEntryManifestDir, cacheEntryManifestFile), registry.manifest, 0o600))

	file, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
	assert.NilError(t, err)

	assert.Equal(t, file, filepath.Join(cache, registry.digest.Hex(), "compose.yaml"))
	content, err := os.ReadFile(file)
	assert.NilError(t, err)
	assert.Equal(t, string(content), string(registry.compose))
}

// The cache holds an entry for a digest when it has a directory named after it
// which holds the manifest that has it, in a directory which an artifact can't
// write: its files have no path separator in their names.
func TestCacheEntryMatchesTheDirectoryHoldingTheManifestOfItsDigest(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2}`)
	dgst := digest.FromBytes(manifest)
	other := digest.FromString("another")

	for name, tt := range map[string]struct {
		dir      string // directory of the entry, if any
		manifest []byte // its manifest, if any
		dgst     digest.Digest
		want     bool
	}{
		"the directory and its manifest": {dgst.Hex(), manifest, dgst, true},
		"no manifest":                    {dgst.Hex(), nil, dgst, false},
		"no directory":                   {"", manifest, dgst, false},
		"a manifest that has not the digest the directory has":          {dgst.Hex(), []byte(`{}`), dgst, false},
		"the manifest of another digest, in a directory named after it": {other.Hex(), manifest, other, false},
		"not a digest": {dgst.Hex(), manifest, digest.Digest("sha256:short"), false},
	} {
		t.Run(name, func(t *testing.T) {
			cache := t.TempDir()
			if tt.dir != "" {
				holder := filepath.Join(cache, tt.dir, cacheEntryManifestDir)
				assert.NilError(t, os.MkdirAll(holder, 0o700))
				if tt.manifest != nil {
					assert.NilError(t, os.WriteFile(filepath.Join(holder, cacheEntryManifestFile), tt.manifest, 0o600))
				}
			}
			assert.Equal(t, cacheEntryMatches(cache, tt.dgst), tt.want)
		})
	}
}

// An artifact which writes a file named as the directory that holds the manifest
// of its entry is refused, rather than have the entry made of what it chose.
func TestWriteCacheEntryManifestRefusesAnArtifactWhichHasTheDirectory(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, cacheEntryManifestDir), []byte("services: {}\n"), 0o600))

	err := writeCacheEntryManifest(dir, []byte(`{"schemaVersion":2}`))
	assert.ErrorContains(t, err, "invalid OCI artifact")
}

// An artifact which has a file named as the directory that holds the manifest of
// its entry, which its annotations let it choose, is refused whole, and nothing of
// it is cached: it can't make an entry of what it wrote.
func TestOCILoaderRefusesAnArtifactWritingTheDirectoryOfTheManifest(t *testing.T) {
	compose := []byte("services:\n  app:\n    image: alpine\n")
	manifest, err := json.Marshal(spec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: oci.ComposeProjectArtifactType,
		Config:       spec.Descriptor{MediaType: oci.ComposeEmptyConfigMediaType, Digest: digest.FromString("{}"), Size: 2},
		Layers: []spec.Descriptor{
			{MediaType: oci.ComposeYAMLMediaType, Digest: digest.FromBytes(compose), Size: int64(len(compose))},
			{
				MediaType: oci.ComposeEnvFileMediaType, Digest: digest.FromBytes(compose), Size: int64(len(compose)),
				Annotations: map[string]string{"com.docker.compose.envfile": cacheEntryManifestDir},
			},
		},
	})
	assert.NilError(t, err)
	registry := &cacheTestRegistry{manifest: manifest, digest: digest.FromBytes(manifest), compose: compose, blob: compose}
	host := registry.serve(t)
	cache := setCacheRoot(t)

	_, err = newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
	assert.ErrorContains(t, err, "invalid OCI artifact")

	entries, err := os.ReadDir(cache)
	assert.Assert(t, err == nil || os.IsNotExist(err))
	assert.Equal(t, len(entries), 0, "nothing is cached")
}

func ptr[T any](v T) *T { return &v }

// fileExists tells whether the file is there, as a regular file.
func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && info.Mode().IsRegular()
}

// entryNames is the sorted names of the entries of a directory.
func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// What the registry reports as the media type of a manifest is not authenticated,
// only its content is: a registry that serves authentic bytes under another type,
// or a manifest with nothing to extract, is refused and caches nothing, rather
// than have an entry of what was not read as the document it is.
func TestOCILoaderRefusesADocumentNotReadAsWhatItIs(t *testing.T) {
	compose := []byte("services:\n  app:\n    image: alpine\n")
	serve := func(t *testing.T, document any, contentType string) (*cacheTestRegistry, string) {
		t.Helper()
		content, err := json.Marshal(document)
		assert.NilError(t, err)
		registry := &cacheTestRegistry{manifest: content, digest: digest.FromBytes(content), compose: compose, blob: compose, contentType: contentType}
		host := registry.serve(t)
		return registry, host
	}
	index := spec.Index{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageIndex,
		ArtifactType: oci.ComposeProjectArtifactType,
		Manifests:    []spec.Descriptor{{MediaType: spec.MediaTypeImageManifest, ArtifactType: oci.ComposeProjectArtifactType, Digest: digest.FromString("child"), Size: 5}},
	}
	emptyManifest := spec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageManifest,
		ArtifactType: oci.ComposeProjectArtifactType,
		Config:       spec.Descriptor{MediaType: oci.ComposeEmptyConfigMediaType, Digest: digest.FromString("{}"), Size: 2},
	}

	for name, tt := range map[string]struct {
		document    any
		contentType string
		err         string
	}{
		"an index reported as a manifest": {index, spec.MediaTypeImageManifest, "which it is not"},
		"a manifest reported as an index": {emptyManifest, spec.MediaTypeImageIndex, "which it is not"},
		"a manifest with no compose file": {emptyManifest, spec.MediaTypeImageManifest, "holds no compose file"},
	} {
		t.Run(name, func(t *testing.T) {
			cache := setCacheRoot(t)
			registry, host := serve(t, tt.document, tt.contentType)

			_, err := newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
			assert.ErrorContains(t, err, tt.err)

			entries, err := os.ReadDir(cache)
			assert.Assert(t, err == nil || os.IsNotExist(err))
			assert.Equal(t, len(entries), 0, "nothing is cached")
		})
	}
}

// What an authentic image index says its manifests are is not enough: a manifest
// it names is verified against the digest it names it by, like the index is, and
// a registry that serves another one for it is refused, with nothing cached.
func TestOCILoaderRefusesAnIndexManifestNotHavingItsDigest(t *testing.T) {
	genuine := newCacheTestRegistry(t)
	tampered := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"` + oci.ComposeProjectArtifactType + `","layers":[]}`)
	content, err := json.Marshal(spec.Index{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    spec.MediaTypeImageIndex,
		ArtifactType: oci.ComposeProjectArtifactType,
		Manifests: []spec.Descriptor{{
			MediaType: spec.MediaTypeImageManifest, ArtifactType: oci.ComposeProjectArtifactType,
			Digest: genuine.digest, Size: int64(len(tampered)),
		}},
	})
	assert.NilError(t, err)
	registry := &cacheTestRegistry{
		manifest: content, digest: digest.FromBytes(content), compose: genuine.compose, blob: genuine.compose,
		contentType: spec.MediaTypeImageIndex,
		byReference: map[digest.Digest][]byte{genuine.digest: tampered},
	}
	host := registry.serve(t)
	cache := setCacheRoot(t)

	_, err = newCacheTestLoader(host).Load(t.Context(), registry.ref(host))
	assert.ErrorContains(t, err, "digest mismatch")

	entries, err := os.ReadDir(cache)
	assert.Assert(t, err == nil || os.IsNotExist(err))
	assert.Equal(t, len(entries), 0, "nothing is cached")
}
