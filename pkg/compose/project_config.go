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

package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client/pkg/versions"
)

// projectConfigMetadataKey is the Docker context metadata key that opts a
// socket into the Compose project-config push. When the current context's
// metadata carries this key set to a truthy value, the socket is assumed to
// accept POST /v{version}/compose/project (see apiVersionComposeProjectConfig),
// and Compose sends the project configuration to the coordinator at the start
// of "compose up" before any other Docker API call.
const projectConfigMetadataKey = "com.docker.compose.project.config"

// projectConfigResponseBodyLimit bounds how much of an error response body is
// read back into the warning message.
const projectConfigResponseBodyLimit = 2048

// projectConfigPushEnabled reports whether the current Docker context opts into
// the project-config push via the projectConfigMetadataKey metadata key. A
// metadata read failure is treated as "not enabled" so that "compose up" is
// never blocked on context inspection. The value is accepted as either a JSON
// boolean true or the string "true" (case-insensitive), mirroring how custom
// context metadata may be decoded (see ConfigFromDockerContext in
// internal/tracing/docker_context.go).
func (s *composeService) projectConfigPushEnabled() bool {
	meta, err := s.dockerCli.ContextStore().GetMetadata(s.dockerCli.CurrentContext())
	if err != nil {
		return false
	}

	var value any
	switch m := meta.Metadata.(type) {
	case command.DockerContext:
		value = m.AdditionalFields[projectConfigMetadataKey]
	case map[string]any:
		value = m[projectConfigMetadataKey]
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

// pushProjectConfig sends the full project configuration to the coordinator
// over the Docker API socket. The request targets the version-negotiated
// POST /v{version}/compose/project endpoint using the engine dialer for
// transport (mirroring internal/desktop/client.go). A non-2xx response is
// returned as an error; callers warn and continue.
func (s *composeService) pushProjectConfig(ctx context.Context, project *types.Project) error {
	payload, err := project.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshaling project config: %w", err)
	}

	version, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return fmt.Errorf("negotiating API version: %w", err)
	}
	if versions.LessThan(version, apiVersionComposeProjectConfig) {
		return fmt.Errorf("coordinator API version %s does not support the project-config push (requires %s or later)",
			version, apiVersionComposeProjectConfig)
	}

	dialer := s.apiClient().Dialer()
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer(ctx)
			},
		},
	}

	// The host is cosmetic: the custom dialer handles routing to the engine
	// socket. It exists only to form a valid URL (see desktop.backendURL).
	url := fmt.Sprintf("http://docker/v%s/compose/project", version)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, projectConfigResponseBodyLimit))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			return fmt.Errorf("coordinator returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("coordinator returned status %d: %s", resp.StatusCode, msg)
	}

	return nil
}
