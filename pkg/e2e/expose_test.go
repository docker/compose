//go:build !windows

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
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// see https://github.com/docker/compose/issues/13378
func TestExposeRange(t *testing.T) {
	c := NewParallelCLI(t)

	f := filepath.Join(t.TempDir(), "compose.yaml")
	err := os.WriteFile(f, []byte(`
name: test-expose-range
services:
  test:
    image: alpine
    expose:
      - "9091-9092"
`), 0o644)
	assert.NilError(t, err)

	t.Cleanup(func() {
		c.cleanupWithDown(t, "test-expose-range")
	})
	c.RunDockerComposeCmd(t, "-f", f, "up")
}
