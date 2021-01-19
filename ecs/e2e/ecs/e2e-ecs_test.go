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
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	. "github.com/docker/compose-cli/utils/e2e"
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
	cmd, testID := setupTest(t)
	secretName := "secret" + testID

	t.Run("create secret", func(t *testing.T) {
		secretFile := filepath.Join(cmd.BinDir, "secret.txt")
		err := ioutil.WriteFile(secretFile, []byte("pass1"), 0644)
		assert.Check(t, err == nil)
		res := cmd.RunDockerCmd("secret", "create", secretName, secretFile)
		assert.Check(t, strings.Contains(res.Stdout(), secretName), res.Stdout())
	})

	t.Run("list secrets", func(t *testing.T) {
		res := cmd.RunDockerCmd("secret", "list")
		assert.Check(t, strings.Contains(res.Stdout(), secretName), res.Stdout())
	})

	t.Run("inspect secret", func(t *testing.T) {
		res := cmd.RunDockerCmd("secret", "inspect", secretName)
		assert.Check(t, strings.Contains(res.Stdout(), `"Name": "`+secretName+`"`), res.Stdout())
	})

	t.Run("rm secret", func(t *testing.T) {
		cmd.RunDockerCmd("secret", "rm", secretName)
		res := cmd.RunDockerCmd("secret", "list")
		assert.Check(t, !strings.Contains(res.Stdout(), secretName), res.Stdout())
	})
}

func TestCompose(t *testing.T) {
	c, stack := setupTest(t)

	t.Run("compose up", func(t *testing.T) {
		c.RunDockerCmd("compose", "up", "--project-name", stack, "-f", "./multi_port_secrets.yaml")
	})

	var webURL, wordsURL, secretsURL string
	t.Run("compose ps", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "--project-name", stack)
		lines := strings.Split(strings.TrimSpace(res.Stdout()), "\n")

		assert.Equal(t, 5, len(lines))

		var dbDisplayed, wordsDisplayed, webDisplayed, secretsDisplayed bool
		for _, line := range lines {
			fields := strings.Fields(line)
			containerID := fields[0]
			serviceName := fields[1]
			switch serviceName {
			case "db":
				dbDisplayed = true
				assert.DeepEqual(t, fields, []string{containerID, serviceName, "Running"})
			case "words":
				wordsDisplayed = true
				assert.Check(t, strings.Contains(fields[3], ":8080->8080/tcp"))
				wordsURL = "http://" + strings.Replace(fields[3], "->8080/tcp", "", 1) + "/noun"
			case "web":
				webDisplayed = true
				assert.Check(t, strings.Contains(fields[3], ":80->80/tcp"))
				webURL = "http://" + strings.Replace(fields[3], "->80/tcp", "", 1)
			case "websecrets":
				secretsDisplayed = true
				assert.Check(t, strings.Contains(fields[3], ":90->90/tcp"))
				secretsURL = "http://" + strings.Replace(fields[3], "->90/tcp", "", 1)
			}
		}

		assert.Check(t, dbDisplayed)
		assert.Check(t, wordsDisplayed)
		assert.Check(t, webDisplayed)
		assert.Check(t, secretsDisplayed)
	})

	t.Run("compose ls", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ls", "--project-name", stack)
		lines := strings.Split(strings.TrimSpace(res.Stdout()), "\n")

		assert.Equal(t, 2, len(lines))
		fields := strings.Fields(lines[1])
		assert.Equal(t, 2, len(fields))
		assert.Equal(t, fields[0], stack)
		assert.Equal(t, "Running", fields[1])
	})

	t.Run("Words GET validating cross service connection", func(t *testing.T) {
		out := HTTPGetWithRetry(t, wordsURL, http.StatusOK, 5*time.Second, 300*time.Second)
		assert.Assert(t, strings.Contains(out, `"word":`))
	})

	t.Run("web app GET", func(t *testing.T) {
		out := HTTPGetWithRetry(t, webURL, http.StatusOK, 3*time.Second, 120*time.Second)
		assert.Assert(t, strings.Contains(out, "Docker Compose demo"))

		out = HTTPGetWithRetry(t, webURL+"/words/noun", http.StatusOK, 2*time.Second, 60*time.Second)
		assert.Assert(t, strings.Contains(out, `"word":`))
	})

	t.Run("access secret", func(t *testing.T) {
		out := HTTPGetWithRetry(t, secretsURL+"/mysecret1", http.StatusOK, 3*time.Second, 120*time.Second)
		out = strings.ReplaceAll(out, "\r", "")
		assert.Equal(t, out, "myPassword1\n")
	})

	t.Run("compose down", func(t *testing.T) {
		cmd := c.NewDockerCmd("compose", "down", "--project-name", stack)
		res := icmd.StartCmd(cmd)

		checkUp := func(t poll.LogT) poll.Result {
			out := res.Stdout()
			if !strings.Contains(out, "DeleteComplete") {
				return poll.Continue("current status \n%s\n", out)
			}
			return poll.Success()
		}
		poll.WaitOn(t, checkUp, poll.WithDelay(2*time.Second), poll.WithTimeout(60*time.Second))
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
			res = c.RunDockerCmd("context", "create", "ecs", contextName, "--profile", localTestProfile)
		} else {
			region := os.Getenv("AWS_DEFAULT_REGION")
			secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
			keyID := os.Getenv("AWS_ACCESS_KEY_ID")
			assert.Check(t, keyID != "")
			assert.Check(t, secretKey != "")
			assert.Check(t, region != "")
			res = c.RunDockerCmd("context", "create", "ecs", contextName, "--from-env")
		}
		res.Assert(t, icmd.Expected{Out: "Successfully created ecs context \"" + contextName + "\""})
		res = c.RunDockerCmd("context", "use", contextName)
		res.Assert(t, icmd.Expected{Out: contextName})
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: contextName + " *"})
	})
	return c, stack
}
