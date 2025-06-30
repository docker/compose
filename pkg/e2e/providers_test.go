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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestDependsOnMultipleProviders(t *testing.T) {
	provider, err := findExecutable("example-provider")
	assert.NilError(t, err)

	path := fmt.Sprintf("%s%s%s", os.Getenv("PATH"), string(os.PathListSeparator), filepath.Dir(provider))
	c := NewParallelCLI(t, WithEnv("PATH="+path))
	const projectName = "depends-on-multiple-providers"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/providers/depends-on-multiple-providers.yaml", "--project-name", projectName, "up")
	res.Assert(t, icmd.Success)
	env := getEnv(res.Combined(), false)
	assert.Check(t, slices.Contains(env, "PROVIDER1_URL=https://magic.cloud/provider1"), env)
	assert.Check(t, slices.Contains(env, "PROVIDER2_URL=https://magic.cloud/provider2"), env)
}

func getEnv(out string, run bool) []string {
	var env []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if !run && strings.HasPrefix(line, "test-1  | ") {
			env = append(env, line[10:])
		}
		if run && strings.Contains(line, "=") && len(strings.Split(line, "=")) == 2 {
			env = append(env, line)
		}
	}
	slices.Sort(env)
	return env
}
