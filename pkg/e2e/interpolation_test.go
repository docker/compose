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
	"bufio"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_interpolation(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "interpolation"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/interpolation/compose.yaml", "--project-name", projectName, "up")
	var env []string
	scanner := bufio.NewScanner(strings.NewReader(res.Stdout()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "test-1  | ") {
			env = append(env, line[10:])
		}
	}
	slices.Sort(env)

	assert.Check(t, slices.Contains(env, "FOO=BAR"))
	assert.Check(t, slices.Contains(env, "ZOT=BAR"))
	assert.Check(t, slices.Contains(env, "QIX=some BAR value"))
	assert.Check(t, slices.Contains(env, "BAR_FROM_ENV_FILE=bar_from_environment"))

}
