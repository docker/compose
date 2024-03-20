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
	"net"
	"net/http"
	"strings"

	"github.com/docker/compose/v2/internal/memnet"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

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

	c := &Client{
		apiEndpoint: apiEndpoint,
		client:      &http.Client{Transport: transport},
	}
	return c
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL("/ping"), http.NoBody)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL("/features"), http.NoBody)
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

// backendURL generates a URL for the given API path.
//
// NOTE: Custom transport handles communication. The host is to create a valid
// URL for the Go http.Client that is also descriptive in error/logs.
func backendURL(path string) string {
	return "http://docker-desktop/" + strings.TrimPrefix(path, "/")
}
