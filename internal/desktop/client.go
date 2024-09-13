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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/docker/compose/v2/internal/memnet"
	"github.com/r3labs/sse"
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

type GetFileSharesConfigResponse struct {
	Active  bool `json:"active"`
	Compose struct {
		ManageBindMounts bool `json:"manageBindMounts"`
	}
}

func (c *Client) GetFileSharesConfig(ctx context.Context) (*GetFileSharesConfigResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL("/mutagen/file-shares/config"), http.NoBody)
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
		return nil, newHTTPStatusCodeError(resp)
	}

	var ret GetFileSharesConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

type CreateFileShareRequest struct {
	HostPath string            `json:"hostPath"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type CreateFileShareResponse struct {
	FileShareID string `json:"fileShareID"`
}

func (c *Client) CreateFileShare(ctx context.Context, r CreateFileShareRequest) (*CreateFileShareResponse, error) {
	rawBody, _ := json.Marshal(r)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backendURL("/mutagen/file-shares"), bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(errBody))
	}
	var ret CreateFileShareResponse
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

type FileShareReceiverState struct {
	TotalReceivedSize uint64 `json:"totalReceivedSize"`
}

type FileShareEndpoint struct {
	Path            string                  `json:"path"`
	TotalFileSize   uint64                  `json:"totalFileSize,omitempty"`
	StagingProgress *FileShareReceiverState `json:"stagingProgress"`
}

type FileShareSession struct {
	SessionID string            `json:"identifier"`
	Alpha     FileShareEndpoint `json:"alpha"`
	Beta      FileShareEndpoint `json:"beta"`
	Labels    map[string]string `json:"labels"`
	Status    string            `json:"status"`
}

func (c *Client) ListFileShares(ctx context.Context) ([]FileShareSession, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL("/mutagen/file-shares"), http.NoBody)
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
		return nil, newHTTPStatusCodeError(resp)
	}

	var ret []FileShareSession
	if err := json.NewDecoder(resp.Body).Decode(&ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func (c *Client) DeleteFileShare(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, backendURL("/mutagen/file-shares/"+id), http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newHTTPStatusCodeError(resp)
	}
	return nil
}

type EventMessage[T any] struct {
	Value T
	Error error
}

func newHTTPStatusCodeError(resp *http.Response) error {
	r := io.LimitReader(resp.Body, 2048)
	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("http status code %d", resp.StatusCode)
	}
	return fmt.Errorf("http status code %d: %s", resp.StatusCode, string(body))
}

func (c *Client) StreamFileShares(ctx context.Context) (<-chan EventMessage[[]FileShareSession], error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backendURL("/mutagen/file-shares/stream"), http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, newHTTPStatusCodeError(resp)
	}

	events := make(chan EventMessage[[]FileShareSession])
	go func(ctx context.Context) {
		defer func() {
			_ = resp.Body.Close()
			for range events {
				// drain the channel
			}
			close(events)
		}()
		if err := readEvents(ctx, resp.Body, events); err != nil {
			select {
			case <-ctx.Done():
			case events <- EventMessage[[]FileShareSession]{Error: err}:
			}
		}
	}(ctx)
	return events, nil
}

func readEvents[T any](ctx context.Context, r io.Reader, events chan<- EventMessage[T]) error {
	eventReader := sse.NewEventStreamReader(r)
	for {
		msg, err := eventReader.ReadEvent()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("reading events: %w", err)
		}
		msg = bytes.TrimPrefix(msg, []byte("data: "))

		var event T
		if err := json.Unmarshal(msg, &event); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case events <- EventMessage[T]{Value: event}:
			// event was sent to channel, read next
		}
	}
}

// backendURL generates a URL for the given API path.
//
// NOTE: Custom transport handles communication. The host is to create a valid
// URL for the Go http.Client that is also descriptive in error/logs.
func backendURL(path string) string {
	return "http://docker-desktop/" + strings.TrimPrefix(path, "/")
}
