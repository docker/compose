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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/plugin"
	dockercli "github.com/docker/cli/cli/command"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
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
	if e2eMode != RunInProcess {
		t.Parallel()
	}
	return NewCLI(t, opts...)
}

// NewCLI creates a CLI instance for running E2E tests.
func NewCLI(t testing.TB, opts ...CLIOption) *CLI {
	t.Helper()

	configDir := t.TempDir()
	initializePlugins(t, configDir)

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
	if os.IsNotExist(err) {
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

func dirContents(dir string) []string {
	var res []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		res = append(res, path)
		return nil
	})
	return res
}

func findExecutable(executableName string) (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	buildPath := filepath.Join(root, "bin", "build")

	bin, err := filepath.Abs(filepath.Join(buildPath, executableName))
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}

	return "", errors.Wrap(os.ErrNotExist, "executable not found")
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
	return "", errors.Wrap(os.ErrNotExist, fmt.Sprintf("plugin not found %s", pluginExecutableName))
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
func (c *CLI) NewCmdWithEnv(envvars []string, cmd string, args ...string) icmd.Cmd {
	// base env -> CLI overrides -> cmd overrides
	cmdEnv := append(c.BaseEnvironment(), c.env...)
	cmdEnv = append(cmdEnv, envvars...)
	return icmd.Cmd{
		Command: append([]string{cmd}, args...),
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
	return c.NewCmd("docker", args...)
}

// RunDockerOrExitError runs a docker command and returns a result
func (c *CLI) RunDockerOrExitError(t testing.TB, args ...string) Result {
	t.Helper()
	t.Logf("\t[%s] docker %s\n", t.Name(), strings.Join(args, " "))
	return c.RunCommand(t, c.NewDockerCmd(t, args...))
}

// RunCmd runs a command, expects no error and returns a result
func (c *CLI) RunCmd(t testing.TB, args ...string) Result {
	t.Helper()
	t.Logf("\t[%s] %s\n", t.Name(), strings.Join(args, " "))
	assert.Assert(t, len(args) >= 1, "require at least one command in parameters")
	res := c.RunCommand(t, c.NewCmd(args[0], args[1:]...))
	res.Assert(t, icmd.Success)
	return res
}

// RunCmdInDir runs a command in a given dir, expects no error and returns a result
func (c *CLI) RunCmdInDir(t testing.TB, dir string, args ...string) Result {
	t.Helper()
	t.Logf("\t[%s] %s\n", t.Name(), strings.Join(args, " "))
	assert.Assert(t, len(args) >= 1, "require at least one command in parameters")
	cmd := c.NewCmd(args[0], args[1:]...)
	cmd.Dir = dir
	if e2eMode == RunInProcess {
		t.Skip("cmd.Dir is not supported running inside test process")
	}
	res := c.RunCommand(t, cmd)
	res.Assert(t, icmd.Success)
	return res
}

// RunDockerCmd runs a docker command, expects no error and returns a result
func (c *CLI) RunDockerCmd(t testing.TB, args ...string) Result {
	t.Helper()
	res := c.RunDockerOrExitError(t, args...)
	assert.Equal(t, res, icmd.Success)
	return res
}

// RunDockerComposeCmd runs a docker compose command, expects no error and returns a result
func (c *CLI) RunDockerComposeCmd(t testing.TB, args ...string) Result {
	t.Helper()
	res := c.RunDockerComposeCmdNoCheck(t, args...)
	res.Assert(t, icmd.Success)
	return res
}

// RunDockerComposeCmdNoCheck runs a docker compose command, don't presume of any expectation and returns a result
func (c *CLI) RunDockerComposeCmdNoCheck(t testing.TB, args ...string) Result {
	t.Helper()
	cmd := c.NewDockerComposeCmd(t, args...)
	cmd.Stdout = os.Stdout
	return c.RunCommand(t, cmd)
}

func (c *CLI) RunCommand(t testing.TB, cmd icmd.Cmd, cmdOperators ...icmd.CmdOp) Result {
	t.Logf("Running command: %s", strings.Join(cmd.Command, " "))
	if cmd.Command[0] == "docker" {
		cmd.Command[0] = DockerExecutableName
	}

	switch e2eMode {
	case RunAsCLIPlugin:
		return icmdResult{icmd.RunCmd(cmd, cmdOperators...)}
	case RunStandalone:
		if len(cmd.Command) > 1 && cmd.Command[1] == "compose" {
			cmd.Command[0] = ComposeStandalonePath(t)
		}
		return icmdResult{icmd.RunCmd(cmd, cmdOperators...)}
	default:
		if len(cmd.Command) > 1 && cmd.Command[1] == "compose" {
			return c.runInProcess(t, cmd, cmdOperators...)
		} else {
			return icmdResult{icmd.RunCmd(cmd, cmdOperators...)}
		}
	}
}

func (c *CLI) runInProcess(t testing.TB, cmd icmd.Cmd, cmdOperators ...icmd.CmdOp) Result {
	for _, op := range cmdOperators {
		op(&cmd)
	}

	for _, s := range c.env {
		parts := strings.SplitN(s, "=", 2)
		t.Setenv(parts[0], parts[1])
	}

	for _, s := range cmd.Env {
		parts := strings.SplitN(s, "=", 2)
		t.Setenv(parts[0], parts[1])
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	dockerCli, err := dockercli.NewDockerCli(dockercli.WithOutputStream(&stdout), dockercli.WithErrorStream(&stderr))
	assert.NilError(t, err)
	composePlugin, meta := compose.MakeRootCommandFn(dockerCli), compose.Metadata()

	os.Args = cmd.Command
	err = plugin.RunPlugin(dockerCli, composePlugin, meta)

	var exitCode int
	if err != nil {
		exitCode = 1
		if sterr, ok := err.(cli.StatusError); ok {
			if sterr.Status != "" {
				fmt.Fprintln(dockerCli.Err(), sterr.Status)
			}
			// StatusError should only be used for errors, and all errors should
			// have a non-zero exit status, so never exit with 0
			exitCode = sterr.StatusCode
			if sterr.StatusCode == 0 {
				exitCode = 1
			}
		}
		fmt.Fprintln(dockerCli.Err(), err)
	}
	return &invocationResult{
		ExitCode: exitCode,
		error:    err,
		Timeout:  false,
		Out:      stdout.String(),
		Err:      stderr.String(),
	}
}

type Result interface {
	fmt.Stringer
	Error() error
	Stdout() string
	Stderr() string
	Combined() string
	Assert(t assert.TestingT, exp icmd.Expected) Result
}

type icmdResult struct {
	*icmd.Result
}

func (i icmdResult) Assert(t assert.TestingT, exp icmd.Expected) Result {
	i.Result.Assert(t, exp)
	return i
}

func (i icmdResult) Error() error {
	return i.Result.Error
}

// invocationResult implements Result for a command ran directly running PluginMain() function in-process
type invocationResult struct {
	ExitCode int
	error    error
	// Timeout is true if the command was killed because it ran for too long
	Timeout bool
	Out     string
	Err     string
}

func (r *invocationResult) Error() error {
	return r.error
}

func (r *invocationResult) Stdout() string {
	return r.Out
}

func (r *invocationResult) Stderr() string {
	return r.Err
}

func (r *invocationResult) Combined() string {
	return r.Out + r.Err
}

func (r *invocationResult) Assert(t assert.TestingT, exp icmd.Expected) Result {
	assert.Assert(t, r.Equal(exp))
	return r
}

func (r *invocationResult) Equal(exp icmd.Expected) cmp.Comparison {
	return func() cmp.Result {
		return cmp.ResultFromError(r.match(exp))
	}
}

func (r *invocationResult) Compare(exp icmd.Expected) error {
	return r.match(exp)
}

func (r *invocationResult) String() string {
	var timeout string
	if r.Timeout {
		timeout = " (timeout)"
	}
	var errString string
	if r.Error() != nil {
		errString = "\nError:    " + r.Error().Error()
	}

	return fmt.Sprintf(`
ExitCode: %d%s%s
Stdout:   %v
Stderr:   %v
`,
		r.ExitCode,
		timeout,
		errString,
		r.Stdout(),
		r.Stderr())
}

func (r *invocationResult) match(exp icmd.Expected) error {
	errs := []string{}
	add := func(format string, args ...interface{}) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if exp.ExitCode != r.ExitCode {
		add("ExitCode was %d expected %d", r.ExitCode, exp.ExitCode)
	}
	if exp.Timeout != r.Timeout {
		if exp.Timeout {
			add("Expected command to timeout")
		} else {
			add("Expected command to finish, but it hit the timeout")
		}
	}
	if !matchOutput(exp.Out, r.Out) {
		add("Expected stdout to contain %q", exp.Out)
	}
	if !matchOutput(exp.Err, r.Err) {
		add("Expected stderr to contain %q", exp.Err)
	}
	switch {
	// If a non-zero exit code is expected there is going to be an error.
	// Don't require an error message as well as an exit code because the
	// error message is going to be "exit status <code> which is not useful
	case exp.Error == "" && exp.ExitCode != 0:
	case exp.Error == "" && r.Error() != nil:
		add("Expected no error")
	case exp.Error != "" && r.Error() == nil:
		add("Expected error to contain %q, but there was no error", exp.Error)
	case exp.Error != "" && !strings.Contains(r.Error().Error(), exp.Error):
		add("Expected error to contain %q", exp.Error)
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nFailures:\n%s", r, strings.Join(errs, "\n"))
}

func matchOutput(expected string, actual string) bool {
	switch expected {
	case icmd.None:
		return actual == ""
	default:
		return strings.Contains(actual, expected)
	}
}

// NewDockerComposeCmd creates a command object for Compose, either in plugin
// or standalone mode (based on build tags).
func (c *CLI) NewDockerComposeCmd(t testing.TB, args ...string) icmd.Cmd {
	t.Helper()
	cargs := append([]string{"compose"}, args...)
	return c.NewDockerCmd(t, cargs...)
}

// ComposeStandalonePath returns the path to the locally-built Compose
// standalone binary from the repo.
//
// This function will fail the test immediately if invoked when not running
// in standalone test mode.
func ComposeStandalonePath(t testing.TB) string {
	t.Helper()
	if e2eMode != RunStandalone {
		require.Fail(t, "Not running in standalone mode")
	}
	composeBinary, err := findExecutable(DockerComposeExecutableName)
	require.NoError(t, err, "Could not find standalone Compose binary (%q)",
		DockerComposeExecutableName)
	return composeBinary
}

// StdoutContains returns a predicate on command result expecting a string in stdout
func StdoutContains(expected string) func(Result) bool {
	return func(res Result) bool {
		return strings.Contains(res.Stdout(), expected)
	}
}

func IsHealthy(service string) func(res Result) bool {
	return func(res Result) bool {
		type state struct {
			Name   string `json:"name"`
			Health string `json:"health"`
		}

		ps := []state{}
		err := json.Unmarshal([]byte(res.Stdout()), &ps)
		if err != nil {
			return false
		}
		for _, state := range ps {
			if state.Name == service && state.Health == "healthy" {
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
	predicate func(Result) bool,
	timeout time.Duration,
	delay time.Duration,
) {
	t.Helper()
	assert.Assert(t, timeout.Nanoseconds() > delay.Nanoseconds(), "timeout must be greater than delay")
	var res Result
	checkStopped := func(logt poll.LogT) poll.Result {
		fmt.Printf("\t[%s] %s\n", t.Name(), strings.Join(command.Command, " "))
		res = c.RunCommand(t, command)
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
