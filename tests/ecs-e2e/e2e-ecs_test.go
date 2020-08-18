/*
   Copyright 2020 Docker, Inc.

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
	"gotest.tools/v3/assert"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/docker/api/tests/framework"
	"gotest.tools/v3/icmd"
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

func TestCompose(t *testing.T) {
	startTime := strconv.Itoa(int(time.Now().UnixNano()))
	c := NewE2eCLI(t, binDir)
	contextName := "teste2e" + startTime
	stack := contextName

	t.Run("create context", func(t *testing.T) {
		profile := contextName
		region := os.Getenv("AWS_DEFAULT_REGION")
		secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		keyID := os.Getenv("AWS_ACCESS_KEY_ID")
		res := c.RunDockerCmd("context", "create", "ecs", contextName, "--profile", profile, "--region", region, "--secret-key", secretKey, "--key-id", keyID)
		res.Assert(t, icmd.Success)
		res = c.RunDockerCmd("context", "use", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: contextName + " *"})
	})

	t.Run("compose up", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "up", "--project-name", stack, "-f", "../composefiles/nginx.yaml")
		res.Assert(t, icmd.Success)
	})


	t.Run("compose ps", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "--project-name", stack)
		res.Assert(t, icmd.Success)
		lines := strings.Split(res.Stdout(), "\n")

		assert.Equal(t,3,  len(lines))
		fields := strings.Fields(lines[1])
		assert.Equal(t, 4, len(fields))
		assert.Check(t, strings.Contains(fields[0], stack))
		assert.Equal(t, "nginx", fields[1])
		assert.Equal(t, "1/1", fields[2])
		assert.Check(t, strings.Contains(fields[3], "->80/http"))
		url := "http://" + strings.Replace(fields[3], "->80/http", "", 1)

		r, err := http.Get(url)
		assert.NilError(t, err)
		assert.Equal(t, r.StatusCode, http.StatusOK)
		b, err := ioutil.ReadAll(r.Body)
		assert.NilError(t, err)
		assert.Assert(t, strings.Contains(string(b), "Welcome to nginx!"))
	})

	t.Run("compose down", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "down", "--project-name", stack, "-f", "../composefiles/nginx.yaml")
		res.Assert(t, icmd.Success)
	})
}
