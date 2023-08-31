/*
   Copyright 2022 Docker Compose CLI authors

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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireServiceState ensures that the container is in the expected state
// (running or exited).
func RequireServiceState(t testing.TB, cli *CLI, service string, state string) {
	t.Helper()
	psRes := cli.RunDockerComposeCmd(t, "ps", "--all", "--format=json", service)
	var svc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(psRes.Stdout()), &svc),
		"Invalid `compose ps` JSON: command output: %s",
		psRes.Combined())

	require.Equal(t, service, svc["Service"],
		"Found ps output for unexpected service")
	require.Equalf(t,
		strings.ToLower(state),
		strings.ToLower(svc["State"].(string)),
		"Service %q (%s) not in expected state",
		service, svc["Name"],
	)
}
