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
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	. "github.com/docker/compose-cli/tests/framework"
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

func TestSecrets(t *testing.T) {
	c, testID := setupTest(t)
	secretName := "secret" + testID
	description := "description " + testID

	t.Run("create secret", func(t *testing.T) {
		res := c.RunDockerCmd("secret", "create", secretName, "-u", "user1", "-p", "pass1", "-d", description)
		res.Assert(t, icmd.Success)
		assert.Check(t, strings.Contains(res.Stdout(), "secret:"+secretName))
	})

	t.Run("list secrets", func(t *testing.T) {
		res := c.RunDockerCmd("secret", "list")
		res.Assert(t, icmd.Success)
		assert.Check(t, strings.Contains(res.Stdout(), secretName))
		assert.Check(t, strings.Contains(res.Stdout(), description))
	})

	t.Run("inspect secret", func(t *testing.T) {
		res := c.RunDockerCmd("secret", "inspect", secretName)
		res.Assert(t, icmd.Success)
		assert.Check(t, strings.Contains(res.Stdout(), `"Name": "`+secretName+`"`))
		assert.Check(t, strings.Contains(res.Stdout(), `"Description": "`+description+`"`))
	})

	t.Run("rm secret", func(t *testing.T) {
		res := c.RunDockerCmd("secret", "rm", secretName)
		res.Assert(t, icmd.Success)
		res = c.RunDockerCmd("secret", "list")
		res.Assert(t, icmd.Success)
		assert.Check(t, !strings.Contains(res.Stdout(), secretName))
	})
}

func TestCompose(t *testing.T) {
	c, stack := setupTest(t)

	t.Run("compose up", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "up", "--project-name", stack, "-f", "../composefiles/nginx.yaml")
		res.Assert(t, icmd.Success)
	})

	var url string
	t.Run("compose ps", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "--project-name", stack)
		res.Assert(t, icmd.Success)
		lines := strings.Split(res.Stdout(), "\n")

		assert.Equal(t, 3, len(lines))
		fields := strings.Fields(lines[1])
		assert.Equal(t, 4, len(fields))
		assert.Check(t, strings.Contains(fields[0], stack))
		assert.Equal(t, "nginx", fields[1])
		assert.Equal(t, "1/1", fields[2])
		assert.Check(t, strings.Contains(fields[3], "->80/http"))
		url = "http://" + strings.Replace(fields[3], "->80/http", "", 1)
	})

	t.Run("nginx GET", func(t *testing.T) {
		checkUp := func(t poll.LogT) poll.Result {
			r, err := http.Get(url)
			if err != nil {
				return poll.Continue("Err while getting %s : %v", url, err)
			} else if r.StatusCode != http.StatusOK {
				return poll.Continue("status %s while getting %s", r.Status, url)
			}
			b, err := ioutil.ReadAll(r.Body)
			if err == nil && strings.Contains(string(b), "Welcome to nginx!") {
				return poll.Success()
			}
			return poll.Error(fmt.Errorf("No nginx welcome page received at %s: \n%s", url, string(b)))
		}
		poll.WaitOn(t, checkUp, poll.WithDelay(2*time.Second), poll.WithTimeout(60*time.Second))
	})

	t.Run("compose down", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "down", "--project-name", stack, "-f", "../composefiles/nginx.yaml")
		res.Assert(t, icmd.Success)
	})
}

func setupTest(t *testing.T) (*E2eCLI, string) {
	startTime := strconv.Itoa(int(time.Now().UnixNano()))
	c := NewParallelE2eCLI(t, binDir)
	contextName := "e2e" + t.Name() + startTime
	stack := contextName
	t.Run("create context", func(t *testing.T) {
		localTestProfile := os.Getenv("TEST_AWS_PROFILE")
		var res *icmd.Result
		if localTestProfile != "" {
			region := os.Getenv("TEST_AWS_REGION")
			assert.Check(t, region != "")
			res = c.RunDockerCmd("context", "create", "ecs", contextName, "--profile", localTestProfile, "--region", region)
			res.Assert(t, icmd.Success)
		} else {
			profile := contextName
			region := os.Getenv("AWS_DEFAULT_REGION")
			secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
			keyID := os.Getenv("AWS_ACCESS_KEY_ID")
			assert.Check(t, keyID != "")
			assert.Check(t, secretKey != "")
			assert.Check(t, region != "")
			res = c.RunDockerCmd("context", "create", "ecs", contextName, "--profile", profile, "--region", region, "--secret-key", secretKey, "--key-id", keyID)
			res.Assert(t, icmd.Success)
		}
		res = c.RunDockerCmd("context", "use", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: contextName + " *"})
	})
	return c, stack
}
