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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRemoveOrphans(t *testing.T) {
	c := NewCLI(t)

	const projectName = "compose-e2e-orphans"

	c.RunDockerComposeCmd(t, "-f", "./fixtures/orphans/compose.yaml", "-p", projectName, "run", "orphan")
	res := c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--all")
	assert.Check(t, strings.Contains(res.Combined(), "compose-e2e-orphans-orphan-run-"))

	c.RunDockerComposeCmd(t, "-f", "./fixtures/orphans/compose.yaml", "-p", projectName, "up", "-d")

	res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--all")
	assert.Check(t, !strings.Contains(res.Combined(), "compose-e2e-orphans-orphan-run-"))
}
