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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_interpolation(t *testing.T) {
	provider, err := findExecutable("example-provider")
	assert.NilError(t, err)
	path := fmt.Sprintf("%s%s%s", os.Getenv("PATH"), string(os.PathListSeparator), filepath.Dir(provider))
	c := NewParallelCLI(t, WithEnv("PATH="+path))

	const projectName = "interpolation"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/interpolation/compose.yaml", "--project-name", projectName, "up")
	env := getEnv(res.Combined())

	assert.Check(t, slices.Contains(env, "FOO=FOO-from-dot-env"))
	assert.Check(t, slices.Contains(env, "ZOT=FOO-from-dot-env"))
	assert.Check(t, slices.Contains(env, "QIX=some FOO-from-dot-env value"))
	assert.Check(t, slices.Contains(env, "BAR_FROM_ENV_FILE=bar_from_environment"))

	assert.Check(t, slices.Contains(env, "BY_PROVIDER_FROM_ENV=https://magic.cloud/example"))
	assert.Check(t, slices.Contains(env, "BY_PROVIDER_FROM_ENV_FILE=https://magic.cloud/example"))
}

func Test_interpolationWithInclude(t *testing.T) {
	provider, err := findExecutable("example-provider")
	assert.NilError(t, err)
	path := fmt.Sprintf("%s%s%s", os.Getenv("PATH"), string(os.PathListSeparator), filepath.Dir(provider))
	c := NewParallelCLI(t, WithEnv("PATH="+path))

	const projectName = "interpolation-include"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/interpolation/include/compose.yaml", "--project-name", projectName, "up")
	env := getEnv(res.Combined())

	assert.Check(t, slices.Contains(env, "FOO=FOO-from-include"))
	assert.Check(t, slices.Contains(env, "ZOT=FOO-from-include"))
	assert.Check(t, slices.Contains(env, "QIX=some FOO-from-include value"))

	assert.Check(t, slices.Contains(env, "BY_PROVIDER_FROM_ENV=https://magic.cloud/example"))
	assert.Check(t, slices.Contains(env, "BY_PROVIDER_FROM_ENV_FILE=https://magic.cloud/example"))
}

func getEnv(out string) []string {
	var env []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "test-1  | ") {
			env = append(env, line[10:])
		}
	}
	slices.Sort(env)
	return env
}
