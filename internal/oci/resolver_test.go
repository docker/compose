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
	"sync/atomic"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"gotest.tools/v3/assert"
)

// recordingRoundTripper counts RoundTrip invocations on a delegate so tests
// can verify a supplied transport is actually used by the resolver.
type recordingRoundTripper struct {
	delegate http.RoundTripper
	calls    atomic.Int32
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
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

func TestNewResolver_NilTransportIsValid(t *testing.T) {
	resolver := NewResolver(&configfile.ConfigFile{}, nil)
	assert.Assert(t, resolver != nil, "NewResolver must return a non-nil resolver when transport is nil")
}
