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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

var (
	// DockerExecutableName is the OS dependent Docker CLI binary name
	DockerExecutableName = "docker"
)

func init() {
	if runtime.GOOS == "windows" {
		DockerExecutableName = DockerExecutableName + ".exe"
	}
}

// E2eCLI is used to wrap the CLI for end to end testing
// nolint stutter
type E2eCLI struct {
	BinDir    string
	ConfigDir string
	test      *testing.T
}

// NewParallelE2eCLI returns a configured TestE2eCLI with t.Parallel() set
func NewParallelE2eCLI(t *testing.T, binDir string) *E2eCLI {
	t.Parallel()
	return newE2eCLI(t, binDir)
}

func newE2eCLI(t *testing.T, binDir string) *E2eCLI {
	d, err := ioutil.TempDir("", "")
	assert.Check(t, is.Nil(err))

	t.Cleanup(func() {
		if t.Failed() {
			conf, _ := ioutil.ReadFile(filepath.Join(d, "config.json"))
			t.Errorf("Config: %s\n", string(conf))
			t.Error("Contents of config dir:")
			for _, p := range dirContents(d) {
				t.Errorf(p)
			}
		}
		_ = os.RemoveAll(d)
	})

	_ = os.MkdirAll(filepath.Join(d, "cli-plugins"), 0755)
	composePluginFile := "docker-compose"
	scanPluginFile := "docker-scan"
	if runtime.GOOS == "windows" {
		composePluginFile += ".exe"
		scanPluginFile += ".exe"
	}
	composePlugin, err := findExecutable(composePluginFile, []string{"../../bin", "../../../bin"})
	if os.IsNotExist(err) {
		fmt.Println("WARNING: docker-compose cli-plugin not found")
	}
	if err == nil {
		err = CopyFile(composePlugin, filepath.Join(d, "cli-plugins", composePluginFile))
		if err != nil {
			panic(err)
		}
		// We don't need a functional scan plugin, but a valid plugin binary
		err = CopyFile(composePlugin, filepath.Join(d, "cli-plugins", scanPluginFile))
		if err != nil {
			panic(err)
		}
	}

	return &E2eCLI{binDir, d, t}
}

func dirContents(dir string) []string {
	res := []string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		res = append(res, filepath.Join(dir, path))
		return nil
	})
	return res
}

func findExecutable(executableName string, paths []string) (string, error) {
	for _, p := range paths {
		bin, err := filepath.Abs(path.Join(p, executableName))
		if err != nil {
			return "", err
		}

		if _, err := os.Stat(bin); os.IsNotExist(err) {
			continue
		}

		return bin, nil
	}

	return "", errors.Wrap(os.ErrNotExist, "executable not found")
}

// CopyFile copies a file from a sourceFile to a destinationFile setting permissions to 0755
func CopyFile(sourceFile string, destinationFile string) error {
	src, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	// nolint: errcheck
	defer src.Close()

	dst, err := os.OpenFile(destinationFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	// nolint: errcheck
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return err
	}

	return err
}

// NewCmd creates a cmd object configured with the test environment set
func (c *E2eCLI) NewCmd(command string, args ...string) icmd.Cmd {
	env := append(os.Environ(),
		"DOCKER_CONFIG="+c.ConfigDir,
		"KUBECONFIG=invalid",
	)
	return icmd.Cmd{
		Command: append([]string{command}, args...),
		Env:     env,
	}
}

// MetricsSocket get the path where test metrics will be sent
func (c *E2eCLI) MetricsSocket() string {
	return filepath.Join(c.ConfigDir, "./docker-cli.sock")
}

// NewDockerCmd creates a docker cmd without running it
func (c *E2eCLI) NewDockerCmd(args ...string) icmd.Cmd {
	return c.NewCmd(DockerExecutableName, args...)
}

// RunDockerOrExitError runs a docker command and returns a result
func (c *E2eCLI) RunDockerOrExitError(args ...string) *icmd.Result {
	fmt.Printf("\t[%s] docker %s\n", c.test.Name(), strings.Join(args, " "))
	return icmd.RunCmd(c.NewDockerCmd(args...))
}

// RunCmd runs a command, expects no error and returns a result
func (c *E2eCLI) RunCmd(args ...string) *icmd.Result {
	fmt.Printf("\t[%s] %s\n", c.test.Name(), strings.Join(args, " "))
	assert.Assert(c.test, len(args) >= 1, "require at least one command in parameters")
	res := icmd.RunCmd(c.NewCmd(args[0], args[1:]...))
	res.Assert(c.test, icmd.Success)
	return res
}

// RunDockerCmd runs a docker command, expects no error and returns a result
func (c *E2eCLI) RunDockerCmd(args ...string) *icmd.Result {
	res := c.RunDockerOrExitError(args...)
	res.Assert(c.test, icmd.Success)
	return res
}

// StdoutContains returns a predicate on command result expecting a string in stdout
func StdoutContains(expected string) func(*icmd.Result) bool {
	return func(res *icmd.Result) bool {
		return strings.Contains(res.Stdout(), expected)
	}
}

// WaitForCmdResult try to execute a cmd until resulting output matches given predicate
func (c *E2eCLI) WaitForCmdResult(command icmd.Cmd, predicate func(*icmd.Result) bool, timeout time.Duration, delay time.Duration) {
	assert.Assert(c.test, timeout.Nanoseconds() > delay.Nanoseconds(), "timeout must be greater than delay")
	var res *icmd.Result
	checkStopped := func(logt poll.LogT) poll.Result {
		fmt.Printf("\t[%s] %s\n", c.test.Name(), strings.Join(command.Command, " "))
		res = icmd.RunCmd(command)
		if !predicate(res) {
			return poll.Continue("Cmd output did not match requirement: %q", res.Combined())
		}
		return poll.Success()
	}
	poll.WaitOn(c.test, checkStopped, poll.WithDelay(delay), poll.WithTimeout(timeout))
}

// WaitForCondition wait for predicate to execute to true
func (c *E2eCLI) WaitForCondition(predicate func() (bool, string), timeout time.Duration, delay time.Duration) {
	checkStopped := func(logt poll.LogT) poll.Result {
		pass, description := predicate()
		if !pass {
			return poll.Continue("Condition not met: %q", description)
		}
		return poll.Success()
	}
	poll.WaitOn(c.test, checkStopped, poll.WithDelay(delay), poll.WithTimeout(timeout))
}

//Lines split output into lines
func Lines(output string) []string {
	return strings.Split(strings.TrimSpace(output), "\n")
}

// HTTPGetWithRetry performs an HTTP GET on an `endpoint`, using retryDelay also as a request timeout.
// In the case of an error or the response status is not the expeted one, it retries the same request,
// returning the response body as a string (empty if we could not reach it)
func HTTPGetWithRetry(t *testing.T, endpoint string, expectedStatus int, retryDelay time.Duration, timeout time.Duration) string {
	var (
		r   *http.Response
		err error
	)
	client := &http.Client{
		Timeout: retryDelay,
	}
	fmt.Printf("\t[%s] GET %s\n", t.Name(), endpoint)
	checkUp := func(t poll.LogT) poll.Result {
		r, err = client.Get(endpoint)
		if err != nil {
			return poll.Continue("reaching %q: Error %s", endpoint, err.Error())
		}
		if r.StatusCode == expectedStatus {
			return poll.Success()
		}
		return poll.Continue("reaching %q: %d != %d", endpoint, r.StatusCode, expectedStatus)
	}
	poll.WaitOn(t, checkUp, poll.WithDelay(retryDelay), poll.WithTimeout(timeout))
	if r != nil {
		b, err := ioutil.ReadAll(r.Body)
		assert.NilError(t, err)
		return string(b)
	}
	return ""
}
