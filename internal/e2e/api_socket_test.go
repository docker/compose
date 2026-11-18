//go:build e2e

/*
   Copyright 2026 Docker Compose CLI authors

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
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// withRegistryCredentials logs the scenario's Docker configuration in to a
// registry that does not exist, whatever the host's own configuration holds:
// use_api_socket hands the credentials over only if there are some, and not
// from a credential store the scenario can't control.
func withRegistryCredentials(s *Scenario) *Scenario {
	s.t.Helper()
	path := filepath.Join(s.cli.ConfigDir, "config.json")
	config := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		assert.NilError(s.t, json.Unmarshal(data, &config))
	}
	delete(config, "credsStore")
	delete(config, "credHelpers")
	config["auths"] = map[string]any{"registry.example.invalid": map[string]string{"auth": "dGVzdDpzZWNyZXQ="}}
	data, err := json.Marshal(config)
	assert.NilError(s.t, err)
	assert.NilError(s.t, os.WriteFile(path, data, 0o600))
	return s
}

// Regression test for https://github.com/docker/compose/issues/14386: a
// container created beforehand and only started by `compose start` got
// DOCKER_CONFIG pointing at /run/secrets/docker, without the config file the
// use_api_socket option is meant to provide there. The file is injected when
// the container starts, from the project given to the start, which did not
// carry the option.
func TestStartCreatedUseAPISocketService(t *testing.T) {
	withRegistryCredentials(NewScenario(t, "start must provide the use_api_socket config file to a container created beforehand")).
		Step("create makes the container without starting it",
			ComposeCmd("create"),
			ServiceState("app", "created")).
		Step("start runs it with the config file in place",
			ComposeCmd("start"),
			Eventually(ServiceState("app", "running"), 10*time.Second),
			ExecOutputContains("app", "[ -f /run/secrets/docker/config.json ] && echo apisocket-config-present", "apisocket-config-present"))
}

// The registry credentials use_api_socket hands to a container are for its root
// only, as with `docker run --use-api-socket`: the config file has mode 0600 and
// belongs to root, so that another user of the container, which may not even be
// allowed to use the socket, cannot read them.
func TestUseAPISocketCredentialsAreForRootOnly(t *testing.T) {
	withRegistryCredentials(NewScenario(t, "use_api_socket credentials must not be readable by a non-root user of the container")).
		Step("the service, which runs as a non-root user, finds the file owned by root with mode 0600",
			ComposeCmd("up", "-d").Within(60*time.Second),
			ServiceState("app", "running"),
			ExecOutputContains("app", "id -u; stat -c '%a %u' /run/secrets/docker/config.json", "1000\n600 0")).
		Step("and cannot read it",
			ComposeCmd("ps"),
			ExecOutputContains("app", "cat /run/secrets/docker/config.json >/dev/null 2>&1 && echo readable || echo unreadable", "unreadable"))
}

// A service which sets DOCKER_CONFIG itself is taken to know where its
// credentials are: it gets the socket, but no credentials are put in it, as
// with `docker run --use-api-socket`.
func TestUseAPISocketHonorsDockerConfig(t *testing.T) {
	withRegistryCredentials(NewScenario(t, "a service setting DOCKER_CONFIG must not get the registry credentials of the host")).
		Step("the service has the socket, its own DOCKER_CONFIG, and no credentials file",
			ComposeCmd("up", "-d").Within(60*time.Second),
			ServiceState("app", "running"),
			ExecOutputContains("app", "echo DOCKER_CONFIG=$DOCKER_CONFIG; [ -S /var/run/docker.sock ] && echo socket-mounted; [ -e /run/secrets/docker/config.json ] || echo no-credentials-file",
				"DOCKER_CONFIG=/elsewhere\nsocket-mounted\nno-credentials-file"))
}
