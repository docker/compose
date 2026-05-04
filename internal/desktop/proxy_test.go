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

package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHTTPProxySocketEndpoint_UnixSocketExists(t *testing.T) {
	dir := t.TempDir()
	cliSock := filepath.Join(dir, "docker-cli.sock")
	proxySock := filepath.Join(dir, "httpproxy.sock")
	mustTouch(t, cliSock)
	mustTouch(t, proxySock)

	got := httpProxySocketEndpoint("unix://" + cliSock)
	assert.Equal(t, got, "unix://"+proxySock)
}

func TestHTTPProxySocketEndpoint_UnixSocketMissing(t *testing.T) {
	// httpproxy.sock deliberately not created — older DD or partial install.
	dir := t.TempDir()
	cliSock := filepath.Join(dir, "docker-cli.sock")
	mustTouch(t, cliSock)

	got := httpProxySocketEndpoint("unix://" + cliSock)
	assert.Equal(t, got, "", "stat miss must fall back so callers do not dial a non-existent socket")
}

func TestHTTPProxySocketEndpoint_WindowsNamedPipe(t *testing.T) {
	got := httpProxySocketEndpoint("npipe://./pipe/dockerCli")
	assert.Equal(t, got, "npipe://./pipe/dockerHttpProxy")
}

func TestHTTPProxySocketEndpoint_EmptyOrUnknown(t *testing.T) {
	assert.Equal(t, httpProxySocketEndpoint(""), "")
	assert.Equal(t, httpProxySocketEndpoint("tcp://localhost:1234"), "")
}

func TestProxyTransport_NilWhenNoDockerDesktop(t *testing.T) {
	assert.Assert(t, ProxyTransport("") == nil,
		"must return nil so callers fall back to their own (e.g. containerd's) default transport")
}

func TestProxyTransport_NilWhenSocketMissing(t *testing.T) {
	// no httpproxy.sock created
	dir := t.TempDir()
	cliSock := filepath.Join(dir, "docker-cli.sock")
	mustTouch(t, cliSock)

	assert.Assert(t, ProxyTransport("unix://"+cliSock) == nil,
		"must return nil when DD endpoint is set but proxy socket is missing, not a transport that would dial a dead socket")
}

func TestProxyTransport_RoutesThroughDockerDesktop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets test path; Windows uses named pipes which os.Stat handles differently")
	}
	dir := t.TempDir()
	cliSock := filepath.Join(dir, "docker-cli.sock")
	proxySock := filepath.Join(dir, "httpproxy.sock")
	mustTouch(t, cliSock)
	mustTouch(t, proxySock)

	got := ProxyTransport("unix://" + cliSock)
	tr, ok := got.(*http.Transport)
	assert.Assert(t, ok, "expected *http.Transport when DD endpoint is set and socket exists")
	assert.Assert(t, tr != http.DefaultTransport, "must be a clone, not DefaultTransport itself")

	// Verify the clone preserved http.DefaultTransport's production
	// settings (timeouts, idle pool, HTTP/2). Compare to the source
	// fields rather than asserting fixed values so this test follows
	// stdlib changes.
	src := http.DefaultTransport.(*http.Transport)
	assert.Equal(t, tr.MaxIdleConns, src.MaxIdleConns)
	assert.Equal(t, tr.IdleConnTimeout, src.IdleConnTimeout)
	assert.Equal(t, tr.TLSHandshakeTimeout, src.TLSHandshakeTimeout)
	assert.Equal(t, tr.ExpectContinueTimeout, src.ExpectContinueTimeout)
	assert.Equal(t, tr.ForceAttemptHTTP2, src.ForceAttemptHTTP2)
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	assert.NilError(t, err)
	assert.NilError(t, f.Close())
}
