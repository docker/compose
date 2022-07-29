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
	"time"

	"gotest.tools/v3/assert"
)

func TestUpWait(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-deps-wait"

	timeout := time.After(30 * time.Second)
	done := make(chan bool)
	go func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/dependencies/deps-completed-successfully.yaml", "--project-name", projectName, "up", "--wait", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-deps-wait-oneshot-1"), res.Combined())
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("test did not finish in time")
	case <-done:
		break
	}

	c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
}
