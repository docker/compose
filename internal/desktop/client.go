/*
   Copyright 2024 Docker Compose CLI authors

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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/moby/moby/client"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/docker/compose/v5/internal"
	"github.com/docker/compose/v5/internal/memnet"
)

// EngineLabel is used to detect that Compose is running with a Docker
// Desktop context. When present, the value is an endpoint address for an
// in-memory socket (AF_UNIX or named pipe).
const EngineLabel = "com.docker.desktop.address"

// FeatureLogsTab is the feature flag name for the Docker Desktop Logs view.
const FeatureLogsTab = "LogsTab"

const logsDeepLink = "docker-desktop://dashboard/logs"

// LogsAppIDMaxLen mirrors the byte-length cap Docker Desktop's URL handler
// applies to the appId query parameter; values longer than this are
// truncated by the receiver, so we trim ahead of time to avoid emitting
// hyperlinks that will be silently shortened. The slice in BuildLogsURL is
// a byte slice — Compose project names are restricted to the ASCII set
// `[a-z0-9_-]` by loader.NormalizeProjectName, so a byte cap and a rune
// cap coincide for any value that could legitimately reach this builder.
const LogsAppIDMaxLen = 256

// BuildLogsURL returns the deep link that opens Docker Desktop's Logs view,
// optionally pre-filtered to a Compose project. An empty appID yields the
// unfiltered URL.
func BuildLogsURL(appID string) string {
	if appID == "" {
		return logsDeepLink
	}
	if len(appID) > LogsAppIDMaxLen {
		appID = appID[:LogsAppIDMaxLen]
	}
	q := url.Values{"appId": []string{appID}}
	return logsDeepLink + "?" + q.Encode()
}

// identify this client in the logs
var userAgent = "compose/" + internal.Version

// Client for integration with Docker Desktop features.
type Client struct {
	apiEndpoint string
	client      *http.Client
}

// NewClient creates a Desktop integration client for the provided in-memory
// socket address (AF_UNIX or named pipe).
func NewClient(apiEndpoint string) *Client {
	var transport http.RoundTripper = &http.Transport{
		DisableCompression: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return memnet.DialEndpoint(ctx, apiEndpoint)
		},
	}
	transport = otelhttp.NewTransport(transport)

	return &Client{
		apiEndpoint: apiEndpoint,
		client:      &http.Client{Transport: transport},
	}
}

func (c *Client) Endpoint() string {
	return c.apiEndpoint
}

// Close releases any open connections.
func (c *Client) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

type PingResponse struct {
	ServerTime int64 `json:"serverTime"`
}

// Ping is a minimal API used to ensure that the server is available.
func (c *Client) Ping(ctx context.Context) (*PingResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/ping", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var ret PingResponse
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

type FeatureFlagResponse map[string]FeatureFlagValue

type FeatureFlagValue struct {
	Enabled bool `json:"enabled"`
}

func (c *Client) FeatureFlags(ctx context.Context) (FeatureFlagResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/features", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var ret FeatureFlagResponse
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// IsFeatureEnabled checks the feature flag (GET /features) for a given
// feature. Returns true when the feature is rolled out.
func (c *Client) IsFeatureEnabled(ctx context.Context, feature string) (bool, error) {
	flags, err := c.FeatureFlags(ctx)
	if err != nil {
		return false, err
	}
	return flags[feature].Enabled, nil
}

// IsFeatureActive reports whether Docker Desktop is the active engine and the
// given feature flag is enabled. Returns false silently on any failure — the
// engine being unreachable, Desktop not being the active engine, or the flag
// being off — so callers can use this as a single gating check.
func IsFeatureActive(ctx context.Context, apiClient client.APIClient, feature string) bool {
	endpoint, err := Endpoint(ctx, apiClient)
	if err != nil || endpoint == "" {
		return false
	}

	c := NewClient(endpoint)
	defer c.Close() //nolint:errcheck

	enabled, err := c.IsFeatureEnabled(ctx, feature)
	if err != nil {
		return false
	}
	return enabled
}

// IsFeatureActiveStandalone is the convenience form of IsFeatureActive for
// callers without an existing engine API client (e.g. the compose plugin hook
// subprocess). It builds a Docker CLI using the ambient environment to
// resolve the active context, then delegates to IsFeatureActive.
func IsFeatureActiveStandalone(ctx context.Context, feature string) bool {
	dockerCli, err := command.NewDockerCli(command.WithCombinedStreams(io.Discard))
	if err != nil {
		return false
	}
	if err := dockerCli.Initialize(cliflags.NewClientOptions()); err != nil {
		return false
	}
	defer dockerCli.Client().Close() //nolint:errcheck

	return IsFeatureActive(ctx, dockerCli.Client(), feature)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, backendURL(path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// backendURL generates a URL for the given API path.
//
// NOTE: Custom transport handles communication. The host is to create a valid
// URL for the Go http.Client that is also descriptive in error/logs.
func backendURL(path string) string {
	return "http://docker-desktop/" + strings.TrimPrefix(path, "/")
}
