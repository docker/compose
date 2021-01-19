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

package main

import (
	"fmt"
	"os"
	"testing"

	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/utils/e2e"
)

const (
	contextName = "ecs-local-test"
)

var binDir string

func TestMain(m *testing.M) {
	p, cleanup, err := SetupExistingCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	binDir = p
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestCreateContext(t *testing.T) {
	c := NewE2eCLI(t, binDir)

	t.Run("create context", func(t *testing.T) {
		c.RunDockerCmd("context", "create", "ecs", contextName, "--local-simulation")
		res := c.RunDockerCmd("context", "use", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: contextName + " *"})
	})
	t.Run("delete context", func(t *testing.T) {
		res := c.RunDockerCmd("context", "use", "default")
		res.Assert(t, icmd.Expected{Out: "default"})

		res = c.RunDockerCmd("context", "rm", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
	})
}
