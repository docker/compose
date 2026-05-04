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
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/memnet"
)

// Endpoint returns the Docker Desktop API socket endpoint advertised via the
// engine info labels, or "" when the active engine is not Docker Desktop.
func Endpoint(ctx context.Context, apiClient client.APIClient) (string, error) {
	res, err := apiClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", err
	}
	for _, l := range res.Info.Labels {
		if k, v, ok := strings.Cut(l, "="); ok && k == EngineLabel {
			return v, nil
		}
	}
	return "", nil
}

// httpProxySocketEndpoint derives Docker Desktop's HTTP proxy socket endpoint
// from a Docker Desktop socket endpoint in the same directory. Returns ""
// when the input is not a recognized form or when the derived unix socket
// does not exist (older DD versions or non-DD installs).
//
// On macOS/Linux: unix:///path/to/Data/docker-cli.sock      → unix:///path/to/Data/httpproxy.sock
// On Windows:    npipe://./pipe/dockerDesktopLinuxEngine    → npipe://./pipe/dockerHttpProxy
func httpProxySocketEndpoint(endpoint string) string {
	if sockPath, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		proxyPath := filepath.Join(filepath.Dir(sockPath), "httpproxy.sock")
		if _, err := os.Stat(proxyPath); err != nil {
			return ""
		}
		return "unix://" + proxyPath
	}
	if strings.HasPrefix(endpoint, "npipe://") {
		return "npipe://./pipe/dockerHttpProxy"
	}
	return ""
}

// ProxyTransport returns an http.RoundTripper that routes traffic through
// Docker Desktop's PAC-aware HTTP proxy when DD exposes the proxy socket,
// or nil when no override is needed (callers should use their own default
// transport in that case — for the OCI resolver this means containerd's
// built-in transport). Pass "" for endpoint when DD is not the active
// engine.
//
// When DD is available, the returned transport is a clone of
// http.DefaultTransport with only Proxy and DialContext overridden, so it
// preserves stdlib timeout, pooling, and HTTP/2 defaults.
func ProxyTransport(endpoint string) http.RoundTripper {
	proxyEndpoint := httpProxySocketEndpoint(endpoint)
	if proxyEndpoint == "" {
		logrus.Debug("Docker Desktop HTTP proxy not available; deferring to caller's default transport")
		return nil
	}
	logrus.Debugf("routing OCI traffic through Docker Desktop HTTP proxy at %s", proxyEndpoint)
	// Clone http.DefaultTransport to inherit stdlib timeout, pool, and
	// HTTP/2 defaults. Type-assertion is guarded since a process may have
	// replaced http.DefaultTransport with a wrapping RoundTripper (e.g.
	// instrumentation libraries); fall back to a fresh transport in that
	// case rather than panicking.
	var tr *http.Transport
	if defaultTr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = defaultTr.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http"})
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return memnet.DialEndpoint(ctx, proxyEndpoint)
	}
	return tr
}

// ProxyTransportFor discovers the Docker Desktop endpoint via apiClient and
// returns the matching transport, or nil when DD is not active or discovery
// fails (so callers fall back to their own default transport).
func ProxyTransportFor(ctx context.Context, apiClient client.APIClient) http.RoundTripper {
	endpoint, err := Endpoint(ctx, apiClient)
	if err != nil {
		logrus.Debugf("could not detect Docker Desktop endpoint, deferring to caller's default transport: %v", err)
		return nil
	}
	return ProxyTransport(endpoint)
}
