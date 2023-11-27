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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	"github.com/docker/compose/v2/cmd/compose"
)

var (
	// DockerExecutableName is the OS dependent Docker CLI binary name
	DockerExecutableName = "docker"

	// DockerComposeExecutableName is the OS dependent Docker CLI binary name
	DockerComposeExecutableName = "docker-" + compose.PluginName

	// DockerScanExecutableName is the OS dependent Docker Scan plugin binary name
	DockerScanExecutableName = "docker-scan"

	// DockerBuildxExecutableName is the Os dependent Buildx plugin binary name
	DockerBuildxExecutableName = "docker-buildx"

	// WindowsExecutableSuffix is the Windows executable suffix
	WindowsExecutableSuffix = ".exe"
)

func init() {
	if runtime.GOOS == "windows" {
		DockerExecutableName += WindowsExecutableSuffix
		DockerComposeExecutableName += WindowsExecutableSuffix
		DockerScanExecutableName += WindowsExecutableSuffix
		DockerBuildxExecutableName += WindowsExecutableSuffix
	}
}

// CLI is used to wrap the CLI for end to end testing
type CLI struct {
	// ConfigDir for Docker configuration (set as DOCKER_CONFIG)
	ConfigDir string

	// HomeDir for tools that look for user files (set as HOME)
	HomeDir string

	// env overrides to apply to every invoked command
	//
	// To populate, use WithEnv when creating a CLI instance.
	env []string
}

// CLIOption to customize behavior for all commands for a CLI instance.
type CLIOption func(c *CLI)

// NewParallelCLI marks the parent test as parallel and returns a CLI instance
// suitable for usage across child tests.
func NewParallelCLI(t *testing.T, opts ...CLIOption) *CLI {
	t.Helper()
	t.Parallel()
	return NewCLI(t, opts...)
}

// NewCLI creates a CLI instance for running E2E tests.
func NewCLI(t testing.TB, opts ...CLIOption) *CLI {
	t.Helper()

	configDir := t.TempDir()
	initializePlugins(t, configDir)
	initializeContextDir(t, configDir)

	c := &CLI{
		ConfigDir: configDir,
		HomeDir:   t.TempDir(),
	}

	for _, opt := range opts {
		opt(c)
	}
	c.RunDockerComposeCmdNoCheck(t, "version")
	return c
}

// WithEnv sets environment variables that will be passed to commands.
func WithEnv(env ...string) CLIOption {
	return func(c *CLI) {
		c.env = append(c.env, env...)
	}
}

// initializePlugins copies the necessary plugin files to the temporary config
// directory for the test.
func initializePlugins(t testing.TB, configDir string) {
	t.Helper()

	t.Cleanup(func() {
		if t.Failed() {
			if conf, err := os.ReadFile(filepath.Join(configDir, "config.json")); err == nil {
				t.Logf("Config: %s\n", string(conf))
			}
			t.Log("Contents of config dir:")
			for _, p := range dirContents(configDir) {
				t.Logf("  - %s", p)
			}
		}
	})

	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "cli-plugins"), 0o755),
		"Failed to create cli-plugins directory")
	composePlugin, err := findExecutable(DockerComposeExecutableName)
	if errors.Is(err, fs.ErrNotExist) {
		t.Logf("WARNING: docker-compose cli-plugin not found")
	}

	if err == nil {
		CopyFile(t, composePlugin, filepath.Join(configDir, "cli-plugins", DockerComposeExecutableName))
		buildxPlugin, err := findPluginExecutable(DockerBuildxExecutableName)
		if err != nil {
			t.Logf("WARNING: docker-buildx cli-plugin not found, using default buildx installation.")
		} else {
			CopyFile(t, buildxPlugin, filepath.Join(configDir, "cli-plugins", DockerBuildxExecutableName))
		}
		// We don't need a functional scan plugin, but a valid plugin binary
		CopyFile(t, composePlugin, filepath.Join(configDir, "cli-plugins", DockerScanExecutableName))
	}
}

func initializeContextDir(t testing.TB, configDir string) {
	dockerUserDir := ".docker/contexts"
	userDir, err := os.UserHomeDir()
	require.NoError(t, err, "Failed to get user home directory")
	userContextsDir := filepath.Join(userDir, dockerUserDir)
	if checkExists(userContextsDir) {
		dstContexts := filepath.Join(configDir, "contexts")
		require.NoError(t, cp.Copy(userContextsDir, dstContexts), "Failed to copy contexts directory")
	}
}

func checkExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirContents(dir string) []string {
	var res []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		res = append(res, path)
		return nil
	})
	return res
}

func findExecutable(executableName string) (string, error) {
	bin := os.Getenv("COMPOSE_E2E_BIN_PATH")
	if bin == "" {
		_, filename, _, _ := runtime.Caller(0)
		buildPath := filepath.Join(filepath.Dir(filename), "..", "..", "bin", "build")
		var err error
		bin, err = filepath.Abs(filepath.Join(buildPath, executableName))
		if err != nil {
			return "", err
		}
	}

	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	return "", fmt.Errorf("looking for %q: %w", bin, fs.ErrNotExist)
}

func findPluginExecutable(pluginExecutableName string) (string, error) {
	dockerUserDir := ".docker/cli-plugins"
	userDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	bin, err := filepath.Abs(filepath.Join(userDir, dockerUserDir, pluginExecutableName))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	return "", fmt.Errorf("plugin not found %s: %w", pluginExecutableName, os.ErrNotExist)
}

// CopyFile copies a file from a sourceFile to a destinationFile setting permissions to 0755
func CopyFile(t testing.TB, sourceFile string, destinationFile string) {
	t.Helper()

	src, err := os.Open(sourceFile)
	require.NoError(t, err, "Failed to open source file: %s")
	//nolint:errcheck
	defer src.Close()

	dst, err := os.OpenFile(destinationFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	require.NoError(t, err, "Failed to open destination file: %s", destinationFile)
	//nolint:errcheck
	defer dst.Close()

	_, err = io.Copy(dst, src)
	require.NoError(t, err, "Failed to copy file: %s", sourceFile)
}

// BaseEnvironment provides the minimal environment variables used across all
// Docker / Compose commands.
func (c *CLI) BaseEnvironment() []string {
	env := []string{
		"HOME=" + c.HomeDir,
		"USER=" + os.Getenv("USER"),
		"DOCKER_CONFIG=" + c.ConfigDir,
		"KUBECONFIG=invalid",
		"PATH=" + os.Getenv("PATH"),
	}
	dockerContextEnv, ok := os.LookupEnv("DOCKER_CONTEXT")
	if ok {
		env = append(env, "DOCKER_CONTEXT="+dockerContextEnv)
	}

	if coverdir, ok := os.LookupEnv("GOCOVERDIR"); ok {
		_, filename, _, _ := runtime.Caller(0)
		root := filepath.Join(filepath.Dir(filename), "..", "..")
		coverdir = filepath.Join(root, coverdir)
		env = append(env, fmt.Sprintf("GOCOVERDIR=%s", coverdir))
	}
	return env
}

// NewCmd creates a cmd object configured with the test environment set
func (c *CLI) NewCmd(command string, args ...string) icmd.Cmd {
	return icmd.Cmd{
		Command: append([]string{command}, args...),
		Env:     append(c.BaseEnvironment(), c.env...),
	}
}

// NewCmdWithEnv creates a cmd object configured with the test environment set with additional env vars
func (c *CLI) NewCmdWithEnv(envvars []string, command string, args ...string) icmd.Cmd {
	// base env -> CLI overrides -> cmd overrides
	cmdEnv := append(c.BaseEnvironment(), c.env...)
	cmdEnv = append(cmdEnv, envvars...)
	return icmd.Cmd{
		Command: append([]string{command}, args...),
		Env:     cmdEnv,
	}
}

// MetricsSocket get the path where test metrics will be sent
func (c *CLI) MetricsSocket() string {
	return filepath.Join(c.ConfigDir, "docker-cli.sock")
}

// NewDockerCmd creates a docker cmd without running it
func (c *CLI) NewDockerCmd(t testing.TB, args ...string) icmd.Cmd {
	t.Helper()
	for _, arg := range args {
		if arg == compose.PluginName {
			t.Fatal("This test called 'RunDockerCmd' for 'compose'. Please prefer 'RunDockerComposeCmd' to be able to test as a plugin and standalone")
		}
	}
	return c.NewCmd(DockerExecutableName, args...)
}

// RunDockerOrExitError runs a docker command and returns a result
func (c *CLI) RunDockerOrExitError(t testing.TB, args ...string) *icmd.Result {
	t.Helper()
	t.Logf("\t[%s] docker %s\n", t.Name(), strings.Join(args, " "))
	return icmd.RunCmd(c.NewDockerCmd(t, args...))
}

// RunCmd runs a command, expects no error and returns a result
func (c *CLI) RunCmd(t testing.TB, args ...string) *icmd.Result {
	t.Helper()
	t.Logf("\t[%s] %s\n", t.Name(), strings.Join(args, " "))
	assert.Assert(t, len(args) >= 1, "require at least one command in parameters")
	res := icmd.RunCmd(c.NewCmd(args[0], args[1:]...))
	res.Assert(t, icmd.Success)
	return res
}

// RunCmdInDir runs a command in a given dir, expects no error and returns a result
func (c *CLI) RunCmdInDir(t testing.TB, dir string, args ...string) *icmd.Result {
	t.Helper()
	t.Logf("\t[%s] %s\n", t.Name(), strings.Join(args, " "))
	assert.Assert(t, len(args) >= 1, "require at least one command in parameters")
	cmd := c.NewCmd(args[0], args[1:]...)
	cmd.Dir = dir
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Success)
	return res
}

// RunDockerCmd runs a docker command, expects no error and returns a result
func (c *CLI) RunDockerCmd(t testing.TB, args ...string) *icmd.Result {
	t.Helper()
	res := c.RunDockerOrExitError(t, args...)
	res.Assert(t, icmd.Success)
	return res
}

// RunDockerComposeCmd runs a docker compose command, expects no error and returns a result
func (c *CLI) RunDockerComposeCmd(t testing.TB, args ...string) *icmd.Result {
	t.Helper()
	res := c.RunDockerComposeCmdNoCheck(t, args...)
	res.Assert(t, icmd.Success)
	return res
}

// RunDockerComposeCmdNoCheck runs a docker compose command, don't presume of any expectation and returns a result
func (c *CLI) RunDockerComposeCmdNoCheck(t testing.TB, args ...string) *icmd.Result {
	t.Helper()
	cmd := c.NewDockerComposeCmd(t, args...)
	cmd.Stdout = os.Stdout
	t.Logf("Running command: %s", strings.Join(cmd.Command, " "))
	return icmd.RunCmd(cmd)
}

// NewDockerComposeCmd creates a command object for Compose, either in plugin
// or standalone mode (based on build tags).
func (c *CLI) NewDockerComposeCmd(t testing.TB, args ...string) icmd.Cmd {
	t.Helper()
	if composeStandaloneMode {
		return c.NewCmd(ComposeStandalonePath(t), args...)
	}
	args = append([]string{"compose"}, args...)
	return c.NewCmd(DockerExecutableName, args...)
}

// ComposeStandalonePath returns the path to the locally-built Compose
// standalone binary from the repo.
//
// This function will fail the test immediately if invoked when not running
// in standalone test mode.
func ComposeStandalonePath(t testing.TB) string {
	t.Helper()
	if !composeStandaloneMode {
		require.Fail(t, "Not running in standalone mode")
	}
	composeBinary, err := findExecutable(DockerComposeExecutableName)
	require.NoError(t, err, "Could not find standalone Compose binary (%q)",
		DockerComposeExecutableName)
	return composeBinary
}

// StdoutContains returns a predicate on command result expecting a string in stdout
func StdoutContains(expected string) func(*icmd.Result) bool {
	return func(res *icmd.Result) bool {
		return strings.Contains(res.Stdout(), expected)
	}
}

func IsHealthy(service string) func(res *icmd.Result) bool {
	return func(res *icmd.Result) bool {
		type state struct {
			Name   string `json:"name"`
			Health string `json:"health"`
		}

		decoder := json.NewDecoder(strings.NewReader(res.Stdout()))
		for decoder.More() {
			ps := state{}
			err := decoder.Decode(&ps)
			if err != nil {
				return false
			}
			if ps.Name == service && ps.Health == "healthy" {
				return true
			}
		}
		return false
	}
}

// WaitForCmdResult try to execute a cmd until resulting output matches given predicate
func (c *CLI) WaitForCmdResult(
	t testing.TB,
	command icmd.Cmd,
	predicate func(*icmd.Result) bool,
	timeout time.Duration,
	delay time.Duration,
) {
	t.Helper()
	assert.Assert(t, timeout.Nanoseconds() > delay.Nanoseconds(), "timeout must be greater than delay")
	var res *icmd.Result
	checkStopped := func(logt poll.LogT) poll.Result {
		fmt.Printf("\t[%s] %s\n", t.Name(), strings.Join(command.Command, " "))
		res = icmd.RunCmd(command)
		if !predicate(res) {
			return poll.Continue("Cmd output did not match requirement: %q", res.Combined())
		}
		return poll.Success()
	}
	poll.WaitOn(t, checkStopped, poll.WithDelay(delay), poll.WithTimeout(timeout))
}

// WaitForCondition wait for predicate to execute to true
func (c *CLI) WaitForCondition(
	t testing.TB,
	predicate func() (bool, string),
	timeout time.Duration,
	delay time.Duration,
) {
	t.Helper()
	checkStopped := func(logt poll.LogT) poll.Result {
		pass, description := predicate()
		if !pass {
			return poll.Continue("Condition not met: %q", description)
		}
		return poll.Success()
	}
	poll.WaitOn(t, checkStopped, poll.WithDelay(delay), poll.WithTimeout(timeout))
}

// Lines split output into lines
func Lines(output string) []string {
	return strings.Split(strings.TrimSpace(output), "\n")
}

// HTTPGetWithRetry performs an HTTP GET on an `endpoint`, using retryDelay also as a request timeout.
// In the case of an error or the response status is not the expected one, it retries the same request,
// returning the response body as a string (empty if we could not reach it)
func HTTPGetWithRetry(
	t testing.TB,
	endpoint string,
	expectedStatus int,
	retryDelay time.Duration,
	timeout time.Duration,
) string {
	t.Helper()
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
		b, err := io.ReadAll(r.Body)
		assert.NilError(t, err)
		return string(b)
	}
	return ""
}
