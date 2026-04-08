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
	"os"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestBackendSocketEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "macOS unix socket",
			input:    "unix:///Users/me/Library/Containers/com.docker.docker/Data/docker-cli.sock",
			expected: "unix:///Users/me/Library/Containers/com.docker.docker/Data/backend.sock",
		},
		{
			name:     "Linux unix socket",
			input:    "unix:///run/desktop/docker-cli.sock",
			expected: "unix:///run/desktop/backend.sock",
		},
		{
			name:     "Windows named pipe",
			input:    "npipe://./pipe/dockerDesktopLinuxEngine",
			expected: "npipe://./pipe/dockerBackendApiServer",
		},
		{
			name:     "unknown scheme passthrough",
			input:    "tcp://localhost:2375",
			expected: "tcp://localhost:2375",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backendSocketEndpoint(tt.input)
			assert.Equal(t, result, tt.expected)
		})
	}
}

func TestClientPing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped in short mode - test connects to Docker Desktop")
	}
	desktopEndpoint := os.Getenv("COMPOSE_TEST_DESKTOP_ENDPOINT")
	if desktopEndpoint == "" {
		t.Skip("Skipping - COMPOSE_TEST_DESKTOP_ENDPOINT not defined")
	}

	client := NewClient(desktopEndpoint)
	t.Cleanup(func() {
		_ = client.Close()
	})

	now := time.Now()

	ret, err := client.Ping(t.Context())
	assert.NilError(t, err)

	serverTime := time.Unix(0, ret.ServerTime)
	assert.Assert(t, now.Before(serverTime))
}
