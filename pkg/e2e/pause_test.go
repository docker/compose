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

package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/icmd"
)

func TestPause(t *testing.T) {
	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("Skipping test on CI... flaky")
	}
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-pause",
		"COMPOSE_FILE=./fixtures/pause/compose.yaml"))

	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", "-v", "--remove-orphans", "-t", "0")
	}
	cleanup()
	t.Cleanup(cleanup)

	// launch both services and verify that they are accessible
	cli.RunDockerComposeCmd(t, "up", "-d")
	urls := map[string]string{
		"a": urlForService(t, cli, "a", 80),
		"b": urlForService(t, cli, "b", 80),
	}
	for _, url := range urls {
		HTTPGetWithRetry(t, url, http.StatusOK, 50*time.Millisecond, 20*time.Second)
	}

	// pause a and verify that it can no longer be hit but b still can
	cli.RunDockerComposeCmd(t, "pause", "a")
	httpClient := http.Client{Timeout: 250 * time.Millisecond}
	resp, err := httpClient.Get(urls["a"])
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "a should no longer respond")
	var netErr net.Error
	errors.As(err, &netErr)
	require.True(t, netErr.Timeout(), "Error should have indicated a timeout")
	HTTPGetWithRetry(t, urls["b"], http.StatusOK, 50*time.Millisecond, 5*time.Second)

	// unpause a and verify that both containers work again
	cli.RunDockerComposeCmd(t, "unpause", "a")
	for _, url := range urls {
		HTTPGetWithRetry(t, url, http.StatusOK, 50*time.Millisecond, 5*time.Second)
	}
}

func TestPauseServiceNotRunning(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-pause-svc-not-running",
		"COMPOSE_FILE=./fixtures/pause/compose.yaml"))

	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", "-v", "--remove-orphans", "-t", "0")
	}
	cleanup()
	t.Cleanup(cleanup)

	// pause a and verify that it can no longer be hit but b still can
	res := cli.RunDockerComposeCmdNoCheck(t, "pause", "a")

	// TODO: `docker pause` errors in this case, should Compose be consistent?
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPauseServiceAlreadyPaused(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-pause-svc-already-paused",
		"COMPOSE_FILE=./fixtures/pause/compose.yaml"))

	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", "-v", "--remove-orphans", "-t", "0")
	}
	cleanup()
	t.Cleanup(cleanup)

	// launch a and wait for it to come up
	cli.RunDockerComposeCmd(t, "up", "--menu=false", "--wait", "a")
	HTTPGetWithRetry(t, urlForService(t, cli, "a", 80), http.StatusOK, 50*time.Millisecond, 10*time.Second)

	// pause a twice - first time should pass, second time fail
	cli.RunDockerComposeCmd(t, "pause", "a")
	res := cli.RunDockerComposeCmdNoCheck(t, "pause", "a")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: "already paused"})
}

func TestPauseServiceDoesNotExist(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-pause-svc-not-exist",
		"COMPOSE_FILE=./fixtures/pause/compose.yaml"))

	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", "-v", "--remove-orphans", "-t", "0")
	}
	cleanup()
	t.Cleanup(cleanup)

	// pause a and verify that it can no longer be hit but b still can
	res := cli.RunDockerComposeCmdNoCheck(t, "pause", "does_not_exist")
	// TODO: `compose down does_not_exist` and similar error, this should too
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func urlForService(t testing.TB, cli *CLI, service string, targetPort int) string {
	t.Helper()
	return fmt.Sprintf(
		"http://localhost:%d",
		publishedPortForService(t, cli, service, targetPort),
	)
}

func publishedPortForService(t testing.TB, cli *CLI, service string, targetPort int) int {
	t.Helper()
	res := cli.RunDockerComposeCmd(t, "ps", "--format=json", service)
	var svc struct {
		Publishers []struct {
			TargetPort    int
			PublishedPort int
		}
	}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout()), &svc),
		"Failed to parse `%s` output", res.Cmd.String())
	for _, pp := range svc.Publishers {
		if pp.TargetPort == targetPort {
			return pp.PublishedPort
		}
	}
	require.Failf(t, "No published port for target port",
		"Target port: %d\nService: %s", targetPort, res.Combined())
	return -1
}
