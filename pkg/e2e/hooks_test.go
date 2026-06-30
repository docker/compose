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
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

// wcLineCount parses the leading integer from a `wc -l <file>` stdout, whose
// shape is "<count> <filename>". Fails the test if the output cannot be parsed.
func wcLineCount(t *testing.T, stdout string) int {
	t.Helper()
	fields := strings.Fields(stdout)
	assert.Assert(t, len(fields) > 0, "expected wc -l output, got: %q", stdout)
	n, err := strconv.Atoi(fields[0])
	assert.NilError(t, err, "expected leading integer in wc -l output, got: %q", stdout)
	return n
}

func TestPostStartHookInError(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-failure"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/poststart/compose-error.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1})
	assert.Assert(t, strings.Contains(res.Combined(), "test hook exited with status 127"), res.Combined())
}

func TestPostStartHookSuccess(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-success"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/poststart/compose-success.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPreStopHookSuccess(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-success"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPreStopHookInError(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-failure"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/prestop/compose-error.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/prestop/compose-error.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	res.Assert(t, icmd.Expected{ExitCode: 1})
	assert.Assert(t, strings.Contains(res.Combined(), "sample hook exited with status 127"))
}

func TestPreStopHookSuccessWithPreviousStop(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-success-with-previous-stop"

	t.Cleanup(func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
		res.Assert(t, icmd.Expected{ExitCode: 0})
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "stop", "sample")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPostStartAndPreStopHook(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-and-pre-stop"

	t.Cleanup(func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
		res.Assert(t, icmd.Expected{ExitCode: 0})
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPreStartHookSuccess(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-start-success"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-success.yaml", "--project-name", projectName, "up", "-d", "--wait")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	// Service should be able to read the file written by the pre_start hook.
	logs := c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-success.yaml", "--project-name", projectName, "logs", "sample")
	assert.Assert(t, strings.Contains(logs.Combined(), "initialized"), logs.Combined())
}

func TestPreStartHookInError(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-start-failure"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-error.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/pre_start/compose-error.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1})
	assert.Assert(t, strings.Contains(res.Combined(), "pre_start"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "17"), res.Combined())

	// The service container should exist but not be running.
	ps := c.RunDockerCmd(t, "ps", "-a", "--filter", "label=com.docker.compose.project="+projectName, "--format", "{{.Names}} {{.State}}")
	assert.Assert(t, strings.Contains(ps.Combined(), "sample"), ps.Combined())
	assert.Assert(t, !strings.Contains(ps.Combined(), "running"), ps.Combined())
}

func TestPreStartHookBuildInheritance(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-start-build"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-build.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "--rmi", "local", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-build.yaml", "--project-name", projectName, "up", "-d", "--wait")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	logs := c.RunDockerComposeCmd(t, "-f", "fixtures/pre_start/compose-build.yaml", "--project-name", projectName, "logs", "sample")
	assert.Assert(t, strings.Contains(logs.Combined(), "built-image-marker"), logs.Combined())
}

func TestPreStartHookIdempotentReUp(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-idempotent"
		composeFile = "fixtures/pre_start/idempotent/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	// First up: hook writes one unique token.
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	// Probe: exactly 1 line in the tokens file.
	probe := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe.Stdout()), 1, "expected 1 token line after first up, got: %s", probe.Stdout())

	// Second up with no spec change: service is already running so the hook must NOT re-run.
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	// Probe again: still exactly 1 line.
	probe2 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe2.Stdout()), 1, "expected 1 token line after idempotent re-up, got: %s", probe2.Stdout())
}

func TestPreStartHookReRunOnSpecChange(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-spec-change"
		composeV1   = "fixtures/pre_start/spec-change/compose.v1.yaml"
		composeV2   = "fixtures/pre_start/spec-change/compose.v2.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeV2, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	// First up with v1 spec: hook appends "v1".
	c.RunDockerComposeCmd(t, "-f", composeV1, "--project-name", projectName, "up", "-d", "--wait")

	// Probe: file contains "v1".
	probe1 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "cat", "/mnt/versions.txt")
	assert.Assert(t, strings.Contains(probe1.Stdout(), "v1"), "expected v1 after first up, got: %s", probe1.Stdout())
	assert.Assert(t, !strings.Contains(probe1.Stdout(), "v2"), "did not expect v2 yet, got: %s", probe1.Stdout())

	// Second up with v2 spec: hook command changed, container recreated, hook runs again and appends "v2".
	c.RunDockerComposeCmd(t, "-f", composeV2, "--project-name", projectName, "up", "-d", "--wait")

	// Probe: file contains both v1 and v2.
	probe2 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "cat", "/mnt/versions.txt")
	assert.Assert(t, strings.Contains(probe2.Stdout(), "v1"), "expected v1 still present, got: %s", probe2.Stdout())
	assert.Assert(t, strings.Contains(probe2.Stdout(), "v2"), "expected v2 appended after spec change, got: %s", probe2.Stdout())
}

func TestPreStartHookForceRecreate(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-force-recreate"
		composeFile = "fixtures/pre_start/idempotent/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	// First up: hook writes one unique token.
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	probe1 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe1.Stdout()), 1, "expected 1 token line after first up, got: %s", probe1.Stdout())

	// Force-recreate: container is rebuilt so the hook must run again.
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--force-recreate", "--wait")

	// Probe: now 2 lines (one from each up).
	probe2 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe2.Stdout()), 2, "expected 2 token lines after --force-recreate, got: %s", probe2.Stdout())
}

func TestPreStartHookMidSequenceFailure(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-mid-failure"
		composeFile = "fixtures/pre_start/mid-failure/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	// Hook 0 succeeds; hook 1 exits with code 17. up must fail.
	res := c.RunDockerComposeCmdNoCheck(t, "-f", composeFile, "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1})

	// Error must point at hook index 1 (not 0) and report exit code 17.
	assert.Assert(t, strings.Contains(res.Combined(), "pre_start[1]"), "expected pre_start[1] in output, got: %s", res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "17"), "expected exit code 17 in output, got: %s", res.Combined())

	// Hook 0 must have run before hook 1 failed: the file must contain "ran-0".
	probe := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "cat", "/mnt/hooks.txt")
	assert.Assert(t, strings.Contains(probe.Stdout(), "ran-0"), "expected hook 0 output in volume, got: %s", probe.Stdout())

	// The service container must exist but not be running.
	ps := c.RunDockerCmd(t, "ps", "-a", "--filter", "label=com.docker.compose.project="+projectName, "--format", "{{.Names}} {{.State}}")
	assert.Assert(t, strings.Contains(ps.Combined(), "sample"), "expected service container in ps output, got: %s", ps.Combined())
	assert.Assert(t, !strings.Contains(ps.Combined(), "running"), "service container must not be running, got: %s", ps.Combined())
}

func TestPreStartHookSequentialOrder(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-sequential"
		composeFile = "fixtures/pre_start/sequential/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	// File must contain A then B in that exact order.
	probe := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "cat", "/mnt/out")
	assert.Equal(t, probe.Stdout(), "A\nB\n", "expected hooks to run in order A then B")
}

func TestPreStartHookNotReRunOnScaleUp(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-scale-up"
		composeFile = "fixtures/pre_start/idempotent/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	probe1 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe1.Stdout()), 1, "expected 1 token after first up, got: %s", probe1.Stdout())

	// Scale up: the new replica must NOT re-run pre_start because another replica is already running.
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--scale", "sample=2", "--wait")

	probe2 := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/tokens.txt")
	assert.Equal(t, wcLineCount(t, probe2.Stdout()), 1, "expected still 1 token after scale-up, got: %s", probe2.Stdout())
}

func TestPreStartHookRunsOnceForScaledService(t *testing.T) {
	c := NewParallelCLI(t)
	const (
		projectName = "hooks-pre-start-scaled"
		composeFile = "fixtures/pre_start/scaled/compose.yaml"
	)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d", "--wait")

	// per_replica: false (default) → hook must run ONCE for the whole service,
	// even with deploy.replicas: 2.
	probe := c.RunDockerCmd(t, "run", "--rm", "-v", projectName+"_data:/mnt", "alpine", "wc", "-l", "/mnt/log")
	assert.Assert(t, strings.HasPrefix(strings.TrimSpace(probe.Stdout()), "1 "),
		"expected hook to run exactly once across replicas, got: %q", probe.Stdout())
}
