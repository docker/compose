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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientPing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped in short mode - test connects to Docker Desktop")
	}
	desktopEndpoint := os.Getenv("COMPOSE_TEST_DESKTOP_ENDPOINT")
	if desktopEndpoint == "" {
		t.Skip("Skipping - COMPOSE_TEST_DESKTOP_ENDPOINT not defined")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := NewClient(desktopEndpoint)
	t.Cleanup(func() {
		_ = client.Close()
	})

	now := time.Now()

	ret, err := client.Ping(ctx)
	require.NoError(t, err)

	serverTime := time.Unix(0, ret.ServerTime)
	require.True(t, now.Before(serverTime))
}
