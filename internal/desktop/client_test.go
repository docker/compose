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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestBuildLogsURL(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		want  string
	}{
		{
			name:  "empty app id yields paramless url",
			appID: "",
			want:  "docker-desktop://dashboard/logs",
		},
		{
			name:  "simple project name",
			appID: "myapp",
			want:  "docker-desktop://dashboard/logs?appId=myapp",
		},
		{
			name:  "name with hyphen and digits is preserved",
			appID: "my-app-2",
			want:  "docker-desktop://dashboard/logs?appId=my-app-2",
		},
		{
			name:  "characters that need percent-encoding are escaped",
			appID: "weird name/with spaces",
			want:  "docker-desktop://dashboard/logs?appId=weird+name%2Fwith+spaces",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, BuildLogsURL(tt.appID), tt.want)
		})
	}
}

func TestBuildLogsURL_TruncatesLongAppID(t *testing.T) {
	long := strings.Repeat("a", LogsAppIDMaxLen+50)
	got := BuildLogsURL(long)
	want := "docker-desktop://dashboard/logs?appId=" + strings.Repeat("a", LogsAppIDMaxLen)
	assert.Equal(t, got, want)
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
