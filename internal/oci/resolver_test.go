/*
   Copyright 2026 Docker Compose CLI authors

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

package oci

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/opencontainers/go-digest"
	spec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

// recordingRoundTripper counts RoundTrip invocations on a delegate so tests
// can verify a supplied transport is actually used by the resolver. It also
// tracks token-endpoint calls separately so tests can assert the authorizer's
// token fetch goes through the same transport.
type recordingRoundTripper struct {
	delegate  http.RoundTripper
	calls     atomic.Int32
	authCalls atomic.Int32
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	if strings.HasSuffix(req.URL.Path, "/token") {
		r.authCalls.Add(1)
	}
	return r.delegate.RoundTrip(req)
}

// TestNewResolver_UsesProvidedTransport guards that the transport passed to
// NewResolver actually carries OCI traffic. The httptest server returns 401
// so the resolver fails fast without real network access.
func TestNewResolver_UsesProvidedTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	host := server.Listener.Addr().String()
	// Bare *http.Transport (Proxy: nil) keeps the test hermetic — delegating
	// to http.DefaultTransport would honor HTTP[S]_PROXY env vars in CI or
	// dev shells and route requests away from our local httptest server.
	rec := &recordingRoundTripper{delegate: &http.Transport{}}

	// Mark the test host insecure so the resolver uses HTTP scheme; this
	// avoids needing a TLS cert chain just to exercise plumbing.
	resolver := NewResolver(&configfile.ConfigFile{}, rec, host)

	// We expect 401, but only care that the request reached our transport.
	_, _, _ = resolver.Resolve(t.Context(), host+"/test/image:latest")

	assert.Assert(t, rec.calls.Load() > 0,
		"resolver did not invoke the supplied transport — wiring is broken")
}

// TestNewResolver_AuthorizerUsesProvidedTransport guards that the docker
// authorizer's token fetch goes through the supplied transport, not
// http.DefaultClient.
func TestNewResolver_AuthorizerUsesProvidedTransport(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"token":"fake","access_token":"fake","expires_in":300}`))
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("Www-Authenticate", `Bearer realm="`+server.URL+`/token",service="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Authenticated retry — fail fast, we only care the auth dance went
		// through the supplied transport.
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	host := server.Listener.Addr().String()
	rec := &recordingRoundTripper{delegate: &http.Transport{}}

	resolver := NewResolver(&configfile.ConfigFile{}, rec, host)
	_, _, _ = resolver.Resolve(t.Context(), host+"/test/image:latest")

	assert.Assert(t, rec.authCalls.Load() > 0,
		"authorizer token fetch did not go through the supplied transport (bypassed via http.DefaultClient)")
}

func TestNewResolver_NilTransportIsValid(t *testing.T) {
	resolver := NewResolver(&configfile.ConfigFile{}, nil)
	assert.Assert(t, resolver != nil, "NewResolver must return a non-nil resolver when transport is nil")
}

// TestGetBlob_FetchesFromBlobsEndpoint guards that layer content is fetched
// straight from the blobs endpoint. Registries answer HEAD/GET on
// manifests/<digest> with a 500 when the digest points to a non-manifest blob
// (they try to JSON-parse it), and containerd >= 2.3 no longer falls back to
// blobs unless the manifests endpoint returned 404 — so resolving a layer
// digest through Resolve() breaks the pull.
func TestGetBlob_FetchesFromBlobsEndpoint(t *testing.T) {
	content := []byte("services:\n  test:\n    image: alpine\n")
	dgst := digest.FromBytes(content)

	var manifestHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v2/test/artifact/manifests/"+dgst.String():
			// mimic distribution: the blob exists but is not a manifest
			manifestHits.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == "/v2/test/artifact/blobs/"+dgst.String():
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	host := server.Listener.Addr().String()
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)

	ref, err := reference.ParseDockerRef(host + "/test/artifact:latest")
	assert.NilError(t, err)

	got, err := GetBlob(t.Context(), resolver, ref, spec.Descriptor{
		MediaType: ComposeYAMLMediaType,
		Digest:    dgst,
		Size:      int64(len(content)),
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, got, content)
	assert.Equal(t, manifestHits.Load(), int32(0),
		"layer fetch must not go through the manifests endpoint")
}

// blobServer serves fixed bytes on the blobs endpoint for the given digest.
func blobServer(t *testing.T, dgst digest.Digest, served []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.WriteHeader(http.StatusOK)
		case "/v2/test/artifact/blobs/" + dgst.String():
			_, _ = w.Write(served)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// TestGetBlob_RejectsDigestMismatch guards that content served by the
// registry is verified against the descriptor digest: GetBlob bypasses
// containerd's content store, so nothing else checks integrity before the
// bytes are written to disk as a Compose file.
func TestGetBlob_RejectsDigestMismatch(t *testing.T) {
	declared := []byte("services:\n  test:\n    image: alpine\n")
	tampered := []byte("services:\n  test:\n    image: evil/image\n")
	dgst := digest.FromBytes(declared)

	host := blobServer(t, dgst, tampered)
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)
	ref, err := reference.ParseDockerRef(host + "/test/artifact:latest")
	assert.NilError(t, err)

	_, err = GetBlob(t.Context(), resolver, ref, spec.Descriptor{
		MediaType: ComposeYAMLMediaType,
		Digest:    dgst,
		Size:      int64(len(tampered)),
	})
	assert.ErrorContains(t, err, "digest mismatch")
}

// TestGetBlob_RejectsSizeMismatch guards that GetBlob never reads more than
// the declared descriptor size, so a rogue registry cannot cause unbounded
// memory allocation.
func TestGetBlob_RejectsSizeMismatch(t *testing.T) {
	content := []byte("services:\n  test:\n    image: alpine\n")
	dgst := digest.FromBytes(content)

	host := blobServer(t, dgst, content)
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)
	ref, err := reference.ParseDockerRef(host + "/test/artifact:latest")
	assert.NilError(t, err)

	_, err = GetBlob(t.Context(), resolver, ref, spec.Descriptor{
		MediaType: ComposeYAMLMediaType,
		Digest:    dgst,
		Size:      int64(len(content)) - 1, // registry serves more than declared
	})
	assert.ErrorType(t, err, errdefs.IsFailedPrecondition)
	assert.ErrorContains(t, err, "read past expected size")
}

// manifestServer is a registry serving content as the manifest of any reference of test/artifact,
// whatever the digest it tells it has: a rogue registry says the digest it wants.
func manifestServer(t *testing.T, claimed digest.Digest, served []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v2/test/artifact/manifests/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", spec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", claimed.String())
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(served)))
			return
		}
		_, _ = w.Write(served)
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// TestGet_RejectsDigestMismatch guards that the manifest served by the registry is verified against the
// digest of the descriptor, whether the reference is a tag (the registry says the digest) or a digest (the
// reference says it): Get bypasses containerd's content store, so nothing else checks it, and callers
// identify the manifest by that digest.
func TestGet_RejectsDigestMismatch(t *testing.T) {
	pinned := digest.FromString("the manifest the reference pins")
	tampered := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"tampered"}`)

	host := manifestServer(t, pinned, tampered)
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)

	for name, named := range map[string]string{
		"tag":    host + "/test/artifact:latest",
		"digest": host + "/test/artifact@" + pinned.String(),
	} {
		t.Run(name, func(t *testing.T) {
			ref, err := reference.ParseDockerRef(named)
			assert.NilError(t, err)
			_, content, err := Get(t.Context(), resolver, ref)
			assert.ErrorContains(t, err, "digest mismatch")
			assert.Assert(t, content == nil)
		})
	}
}

// TestGet_ReturnsVerifiedManifest guards the regular case: a manifest that has the digest it is served
// under is returned with its descriptor.
func TestGet_ReturnsVerifiedManifest(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.docker.compose.project"}`)
	dgst := digest.FromBytes(manifest)

	host := manifestServer(t, dgst, manifest)
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)

	for name, named := range map[string]string{
		"tag":    host + "/test/artifact:latest",
		"digest": host + "/test/artifact@" + dgst.String(),
	} {
		t.Run(name, func(t *testing.T) {
			ref, err := reference.ParseDockerRef(named)
			assert.NilError(t, err)
			descriptor, content, err := Get(t.Context(), resolver, ref)
			assert.NilError(t, err)
			assert.Equal(t, descriptor.Digest, dgst)
			assert.DeepEqual(t, content, manifest)
		})
	}
}

// TestVerifyDigest guards what is checked against a digest: the algorithms of the
// digests Compose can be given, and a digest which is not one, which is refused
// rather than compared.
func TestVerifyDigest(t *testing.T) {
	content := []byte("content")

	for name, tt := range map[string]struct {
		expected digest.Digest
		err      string
	}{
		"sha256":                      {digest.SHA256.FromBytes(content), ""},
		"sha384":                      {digest.SHA384.FromBytes(content), ""},
		"sha512":                      {digest.SHA512.FromBytes(content), ""},
		"another content (sha256)":    {digest.SHA256.FromString("another"), "digest mismatch"},
		"another content (sha512)":    {digest.SHA512.FromString("another"), "digest mismatch"},
		"an unknown algorithm":        {digest.Digest("md5:" + strings.Repeat("0", 32)), "invalid digest"},
		"a digest which is too short": {digest.Digest("sha256:short"), "invalid digest"},
		"no digest":                   {"", "invalid digest"},
	} {
		t.Run(name, func(t *testing.T) {
			err := verifyDigest("manifest", tt.expected, content)
			if tt.err == "" {
				assert.NilError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.err)
		})
	}
}

// TestGet_RejectsAnInvalidDigest guards that a manifest the registry says has a
// digest which is not one is refused, not compared to it.
func TestGet_RejectsAnInvalidDigest(t *testing.T) {
	manifest := []byte(`{"schemaVersion":2}`)
	host := manifestServer(t, digest.Digest("sha256:short"), manifest)
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)
	ref, err := reference.ParseDockerRef(host + "/test/artifact:latest")
	assert.NilError(t, err)

	_, content, err := Get(t.Context(), resolver, ref)
	assert.Assert(t, err != nil)
	assert.Assert(t, content == nil)
}

// TestGet_RejectsAManifestLargerThanAnnounced guards that no more than the size
// the registry announced is read, which Get relies on to not buffer what a rogue
// registry serves: one which announces a small manifest and serves a larger one is
// refused before anything is hashed or used.
func TestGet_RejectsAManifestLargerThanAnnounced(t *testing.T) {
	announced := []byte(`{"schemaVersion":2}`)
	served := append(slices.Clone(announced), []byte(strings.Repeat(" ", 4096))...)
	dgst := digest.FromBytes(announced)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			return
		}
		w.Header().Set("Content-Type", spec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", dgst.String())
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(announced)))
			return
		}
		_, _ = w.Write(served)
	}))
	t.Cleanup(server.Close)
	host := server.Listener.Addr().String()
	resolver := NewResolver(&configfile.ConfigFile{}, &http.Transport{}, host)
	ref, err := reference.ParseDockerRef(host + "/test/artifact@" + dgst.String())
	assert.NilError(t, err)

	_, content, err := Get(t.Context(), resolver, ref)
	assert.ErrorContains(t, err, "read past expected size")
	assert.Assert(t, content == nil)
}
