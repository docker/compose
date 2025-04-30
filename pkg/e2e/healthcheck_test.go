/*
   Copyright 2023 Docker Compose CLI authors

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
	"gotest.tools/v3/icmd"
)

func TestStartInterval(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-interval"

	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/start_interval/compose.yaml", "--project-name", projectName, "up", "--wait", "-d", "error")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: "healthcheck.start_interval requires healthcheck.start_period to be set"})

	timeout := time.After(30 * time.Second)
	done := make(chan bool)
	go func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/start_interval/compose.yaml", "--project-name", projectName, "up", "--wait", "-d", "test")
		out := res.Combined()
		assert.Assert(t, strings.Contains(out, "Healthy"), out)
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("test did not finish in time")
	case <-done:
		break
	}
}
