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

// Package coordinator holds the coordinator-specific integration used by
// Compose: detecting a coordinator-enabled Docker context and pushing the
// project configuration to it over the engine socket. It mirrors the
// self-contained layout of internal/desktop (detection, HTTP client, versioned
// URL and outcome handling) so that pkg/compose keeps only a thin call-site.
package coordinator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client/pkg/versions"
)

// MetadataKey is the Docker context metadata key that identifies a coordinator
// behind a Docker API socket. When the current context's metadata carries this
// key set to a truthy value, the socket is assumed to accept POST
// /v{version}/compose/project (see MinAPIVersion), and Compose sends the
// project configuration to the coordinator at the start of "compose up" before
// any other Docker API call.
const MetadataKey = "com.docker.compose.coordinator"

// MinAPIVersion is the Engine/coordinator API version that introduced
// POST /v{version}/compose/project for the Compose project-config push. It
// documents the minimum coordinator version and is the single place the push
// gates on.
const MinAPIVersion = "1.51"

// defaultTimeout bounds a single project-config push. The push runs before any
// other Docker API call in "compose up", so without a deadline a coordinator
// that accepts the connection but never responds would hang "up" indefinitely,
// defeating the non-fatal "warn and continue" guarantee.
const defaultTimeout = 10 * time.Second

// responseBodyLimit bounds how much of an error response body is read back
// into the returned error message.
const responseBodyLimit = 2048

// CompleteHeader signals whether the pushed project is the whole project or a
// subset. "compose up" with no service arguments resolves the entire project
// and sends "true"; "compose up <service...>" narrows the project to the named
// services plus their dependency closure and sends "false", telling the
// coordinator to merge the payload with previously pushed config rather than
// treat it as authoritative. An absent header (older Compose clients) must be
// read as "false": those clients may also have pushed a subset, so the
// coordinator must never prune on their behalf.
const CompleteHeader = "X-Compose-Project-Complete"

// Enabled reports whether the current Docker context represents a compose
// coordinator via the MetadataKey metadata key. A metadata read failure is
// treated as "not enabled" so that "compose up" is never blocked on context
// inspection. The value is accepted as either a JSON boolean true or the string
// "true" (case-insensitive), mirroring how custom context metadata may be
// decoded (see ConfigFromDockerContext in internal/tracing/docker_context.go).
func Enabled(dockerCli command.Cli) bool {
	meta, err := dockerCli.ContextStore().GetMetadata(dockerCli.CurrentContext())
	if err != nil {
		return false
	}

	var value any
	switch m := meta.Metadata.(type) {
	case command.DockerContext:
		value = m.AdditionalFields[MetadataKey]
	case map[string]any:
		value = m[MetadataKey]
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

// Client pushes the Compose project configuration to a coordinator over the
// Docker engine socket, using the engine dialer for transport (mirroring
// internal/desktop/client.go).
type Client struct {
	dialer  func(ctx context.Context) (net.Conn, error)
	timeout time.Duration
}

// NewClient builds a coordinator client that reaches the engine over the given
// dialer (typically apiClient.Dialer()).
func NewClient(dialer func(ctx context.Context) (net.Conn, error)) *Client {
	return &Client{dialer: dialer, timeout: defaultTimeout}
}

// PushProjectConfig sends the full project configuration to the coordinator's
// version-negotiated POST /v{version}/compose/project endpoint. apiVersion is
// the negotiated engine API version; the push is rejected before any network
// call when it predates MinAPIVersion. A non-2xx response is returned as an
// error; callers warn and continue. The request is bounded by a timeout so a
// coordinator that never responds cannot hang "compose up".
//
// complete reports whether project is the whole project (true) or a subset
// that the coordinator should merge with what it already holds (false); it is
// conveyed via CompleteHeader.
func (c *Client) PushProjectConfig(ctx context.Context, apiVersion string, project *types.Project, complete bool) error {
	if versions.LessThan(apiVersion, MinAPIVersion) {
		return fmt.Errorf("coordinator API version %s does not support the project-config push (requires %s or later)",
			apiVersion, MinAPIVersion)
	}

	payload, err := project.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshaling project config: %w", err)
	}

	httpClient := &http.Client{
		Timeout: c.timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return c.dialer(ctx)
			},
		},
	}

	// The host is cosmetic: the custom dialer handles routing to the engine
	// socket. It exists only to form a valid URL (see desktop.backendURL).
	url := fmt.Sprintf("http://docker/v%s/compose/project", apiVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Always set the header explicitly (not omit-on-false) so the value is
	// unambiguous on the wire; only absence means "older client".
	req.Header.Set(CompleteHeader, strconv.FormatBool(complete))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
		// The transport is discarded after this call; tear down its idle
		// persistConn goroutines eagerly instead of waiting out
		// IdleConnTimeout, which matters when "up" is invoked in a loop.
		httpClient.CloseIdleConnections()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			return fmt.Errorf("coordinator returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("coordinator returned status %d: %s", resp.StatusCode, msg)
	}

	return nil
}
