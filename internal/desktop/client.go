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
	"path/filepath"
	"strings"

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

// SettingValue represents a Docker Desktop setting with a locked flag and a value.
type SettingValue struct {
	Locked bool `json:"locked"`
	Value  bool `json:"value"`
}

// DesktopSettings represents the "desktop" section of Docker Desktop settings.
type DesktopSettings struct {
	EnableLogsTab SettingValue `json:"enableLogsTab"`
}

// SettingsResponse represents the Docker Desktop settings response.
type SettingsResponse struct {
	Desktop DesktopSettings `json:"desktop"`
}

// Settings fetches the Docker Desktop application settings.
func (c *Client) Settings(ctx context.Context) (*SettingsResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/app/settings", http.NoBody)
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

	var ret SettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

// IsFeatureEnabled checks both the feature flag (GET /features) and the user
// setting (GET /app/settings) for a given feature. Returns true only when the
// feature is both rolled out and enabled by the user. Features without a
// corresponding setting entry are considered enabled if the flag is set.
func (c *Client) IsFeatureEnabled(ctx context.Context, feature string) (bool, error) {
	flags, err := c.FeatureFlags(ctx)
	if err != nil {
		return false, err
	}
	if !flags[feature].Enabled {
		return false, nil
	}

	check, hasCheck := featureSettingChecks[feature]
	if !hasCheck {
		// No setting to verify — feature flag alone is sufficient
		return true, nil
	}

	// The /app/settings endpoint is served by the backend socket, not the
	// docker-cli socket. Derive the backend socket path from the current
	// endpoint.
	backendEndpoint := backendSocketEndpoint(c.apiEndpoint)
	backendCli := NewClient(backendEndpoint)
	defer backendCli.Close() //nolint:errcheck

	settings, err := backendCli.Settings(ctx)
	if err != nil {
		return false, err
	}
	return check(settings), nil
}

// backendSocketEndpoint derives the Docker Desktop backend socket endpoint
// from any socket endpoint in the same directory.
//
// On macOS/Linux: unix:///path/to/Data/docker-cli.sock → unix:///path/to/Data/backend.sock
// On Windows:     npipe://./pipe/dockerDesktopLinuxEngine → npipe://./pipe/dockerBackendApiServer
func backendSocketEndpoint(endpoint string) string {
	if sockPath, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		return "unix://" + filepath.Join(filepath.Dir(sockPath), "backend.sock")
	}
	if _, ok := strings.CutPrefix(endpoint, "npipe://"); ok {
		return "npipe://./pipe/dockerBackendApiServer"
	}
	return endpoint
}

// featureSettingChecks maps feature flag names to their corresponding
// Docker Desktop setting check functions.
var featureSettingChecks = map[string]func(*SettingsResponse) bool{
	FeatureLogsTab: func(s *SettingsResponse) bool {
		return s.Desktop.EnableLogsTab.Value
	},
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
