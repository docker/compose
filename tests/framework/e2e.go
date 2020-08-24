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

package framework

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"

	"github.com/docker/api/containers"
)

var (
	// DockerExecutableName is the OS dependent Docker CLI binary name
	DockerExecutableName   = "docker"
	existingExectuableName = "com.docker.cli"
)

func init() {
	if runtime.GOOS == "windows" {
		DockerExecutableName = DockerExecutableName + ".exe"
		existingExectuableName = existingExectuableName + ".exe"
	}
}

// E2eCLI is used to wrap the CLI for end to end testing
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

// NewE2eCLI returns a configured TestE2eCLI
func NewE2eCLI(t *testing.T, binDir string) *E2eCLI {
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

// SetupExistingCLI copies the existing CLI in a temporary directory so that the
// new CLI can be configured to use it
func SetupExistingCLI() (string, func(), error) {
	p, err := exec.LookPath(existingExectuableName)
	if err != nil {
		p, err = exec.LookPath(DockerExecutableName)
		if err != nil {
			return "", nil, errors.New("existing CLI not found in PATH")
		}
	}
	d, err := ioutil.TempDir("", "")
	if err != nil {
		return "", nil, err
	}
	if err := CopyFile(p, filepath.Join(d, existingExectuableName)); err != nil {
		return "", nil, err
	}
	bin, err := filepath.Abs("../../bin/" + DockerExecutableName)
	if err != nil {
		return "", nil, err
	}
	if err := CopyFile(bin, filepath.Join(d, DockerExecutableName)); err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(d)
	}
	return d, cleanup, nil
}

// CopyFile copies a file from a path to a path setting permissions to 0777
func CopyFile(sourceFile string, destinationFile string) error {
	input, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(destinationFile, input, 0777)
	if err != nil {
		return err
	}
	return nil
}

// NewCmd creates a cmd object configured with the test environment set
func (c *E2eCLI) NewCmd(command string, args ...string) icmd.Cmd {
	path := c.BinDir + ":" + os.Getenv("PATH")
	if runtime.GOOS == "windows" {
		path = c.BinDir + ";" + os.Getenv("PATH")
	}
	env := append(os.Environ(),
		"DOCKER_CONFIG="+c.ConfigDir,
		"KUBECONFIG=invalid",
		"PATH="+path,
	)
	return icmd.Cmd{
		Command: append([]string{command}, args...),
		Env:     env,
	}
}

// NewDockerCmd creates a docker cmd without running it
func (c *E2eCLI) NewDockerCmd(args ...string) icmd.Cmd {
	return c.NewCmd(filepath.Join(c.BinDir, DockerExecutableName), args...)
}

// RunDockerOrExitError runs a docker command and returns a result
func (c *E2eCLI) RunDockerOrExitError(args ...string) *icmd.Result {
	fmt.Printf("	[%s] docker %s\n", c.test.Name(), strings.Join(args, " "))
	return icmd.RunCmd(c.NewDockerCmd(args...))
}

// RunDockerCmd runs a docker command, expects no error and returns a result
func (c *E2eCLI) RunDockerCmd(args ...string) *icmd.Result {
	res := c.RunDockerOrExitError(args...)
	res.Assert(c.test, icmd.Success)
	return res
}

// GoldenFile golden file specific to platform
func GoldenFile(name string) string {
	if runtime.GOOS == "windows" {
		return name + "-windows.golden"
	}
	return name + ".golden"
}

// ParseContainerInspect parses the output of a `docker inspect` command for a
// container
func ParseContainerInspect(stdout string) (*containers.Container, error) {
	var res containers.Container
	rdr := bytes.NewReader([]byte(stdout))
	if err := json.NewDecoder(rdr).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}
