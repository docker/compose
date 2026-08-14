/*
   Copyright 2026 Docker Compose CLI authors

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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"gotest.tools/v3/icmd"
)

// Scenario is a thin declarative layer over the e2e CLI helpers: a test reads
// as a compose.yaml, a sequence of steps (command + expected observables) and
// nothing else. Project naming, cleanup and failure diagnostics (transcript,
// project state, engine events, container logs) are handled by the framework.
//
// Steps execute eagerly: each Step call runs its command, snapshots the
// project containers and evaluates the checks, failing the test with a full
// scenario report on the first unmet expectation.
type Scenario struct {
	t        *testing.T
	cli      *CLI
	intent   string
	project  string
	file     string
	env      []string
	parallel bool
	start    time.Time
	deferred []Action
	steps    []stepRecord
	snaps    []snapshot
}

type stepRecord struct {
	name     string
	command  string
	result   *icmd.Result
	duration time.Duration
}

// containerState is the per-container state captured after each step, used by
// cross-step checks such as NotRecreated or LabelUnchanged.
type containerState struct {
	ID     string
	Name   string
	Labels map[string]string
}

// snapshot maps a service name to its containers, sorted by name.
type snapshot map[string][]containerState

// ScenarioOption customizes a Scenario at creation time.
type ScenarioOption func(*Scenario)

// Serial disables the default parallel execution, for scenarios that mutate
// shared daemon state (e.g. removing images other tests may pull).
func Serial() ScenarioOption {
	return func(s *Scenario) { s.parallel = false }
}

// NewScenario creates a scenario named after the test, with a unique project
// name, an isolated CLI instance and automatic `down` cleanup registered.
// The intent is a one-line statement of the behavior being locked, displayed
// in logs and failure reports.
func NewScenario(t *testing.T, intent string, opts ...ScenarioOption) *Scenario {
	t.Helper()
	s := &Scenario{
		t:        t,
		intent:   intent,
		parallel: true,
		project:  projectNameFor(t.Name()),
		start:    time.Now(),
		snaps:    []snapshot{{}},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.parallel {
		s.cli = NewParallelCLI(t)
	} else {
		s.cli = NewCLI(t)
	}
	t.Logf("scenario: %s (project %s)", intent, s.project)

	// start from — and return to — a clean slate, whatever previous runs left
	s.cli.RunDockerComposeCmdNoCheck(t, "--project-name", s.project, "down", "-v", "--remove-orphans", "--timeout", "0")
	t.Cleanup(func() {
		s.cli.RunDockerComposeCmdNoCheck(t, "--project-name", s.project, "down", "-v", "--remove-orphans", "--timeout", "0")
		for _, action := range s.deferred {
			_ = icmd.RunCmd(s.command(action))
		}
	})
	return s
}

// projectNameFor derives a valid, readable compose project name from a test
// name: TestUpDryRunMissingImage -> e2e-up-dry-run-missing-image.
func projectNameFor(testName string) string {
	name := strings.TrimPrefix(testName, "Test")
	var b strings.Builder
	var prev rune
	for i, r := range name {
		switch {
		case unicode.IsUpper(r):
			if i > 0 && !unicode.IsUpper(prev) {
				b.WriteRune('-')
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
		prev = r
	}
	return "e2e-" + strings.Trim(b.String(), "-")
}

// CLI exposes the underlying CLI instance for the rare setup logic the
// declarative layer doesn't cover.
func (s *Scenario) CLI() *CLI { return s.cli }

// Compose declares the project's compose model, written to a temporary
// directory so the whole scenario is self-contained in the test source.
func (s *Scenario) Compose(yaml string) *Scenario {
	s.t.Helper()
	dir := s.t.TempDir()
	s.file = filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(s.file, []byte(yaml), 0o644); err != nil {
		s.t.Fatalf("failed to write compose.yaml: %v", err)
	}
	return s
}

// Env sets environment variables applied to every subsequent step command
// (and interpolated in the compose model).
func (s *Scenario) Env(kv ...string) *Scenario {
	s.env = append(s.env, kv...)
	return s
}

// Requires skips the scenario unless every requirement is met by the target
// environment.
func (s *Scenario) Requires(reqs ...Requirement) *Scenario {
	s.t.Helper()
	for _, req := range reqs {
		if reason := req(s.t, s.cli); reason != "" {
			s.t.Skip(reason)
		}
	}
	return s
}

// Defer registers a best-effort cleanup action executed after the project is
// taken down, e.g. removing images the scenario pulled.
func (s *Scenario) Defer(actions ...Action) *Scenario {
	s.deferred = append(s.deferred, actions...)
	return s
}

// NonNativePlatform returns a linux platform different from the daemon's,
// for scenarios exercising platform-pinned services without emulation.
func (s *Scenario) NonNativePlatform() string {
	s.t.Helper()
	arch := s.cli.RunDockerCmd(s.t, "info", "--format", "{{.Architecture}}").Stdout()
	if strings.Contains(arch, "x86_64") {
		return "linux/arm64"
	}
	return "linux/amd64"
}

// Step runs an action and asserts the expected observables. The command must
// succeed unless the action is marked MayFail. On the first unmet
// expectation the scenario fails with a transcript and project diagnostics.
func (s *Scenario) Step(name string, action Action, checks ...Check) *Scenario {
	t := s.t
	t.Helper()
	cmd := s.command(action)
	t.Logf("step: %s — %s", name, strings.Join(cmd.Command, " "))

	begin := time.Now()
	res := icmd.RunCmd(cmd)
	rec := stepRecord{name: name, command: strings.Join(cmd.Command, " "), result: res, duration: time.Since(begin)}
	s.steps = append(s.steps, rec)

	if !action.mayFail && res.ExitCode != 0 {
		s.fail(fmt.Errorf("command exited with code %d", res.ExitCode))
	}

	s.snaps = append(s.snaps, s.snapshot())
	ctx := &CheckContext{
		scenario: s,
		result:   res,
		prev:     s.snaps[len(s.snaps)-2],
		curr:     s.snaps[len(s.snaps)-1],
	}
	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			s.fail(fmt.Errorf("expected %s: %w", check.name, err))
		}
	}
	return s
}

// command materializes an action into a runnable command, layering scenario
// env then action env on top of the CLI environment.
func (s *Scenario) command(action Action) icmd.Cmd {
	s.t.Helper()
	var cmd icmd.Cmd
	switch action.kind {
	case kindCompose:
		args := []string{}
		if s.file != "" {
			args = append(args, "-f", s.file)
		}
		args = append(args, "--project-name", s.project)
		args = append(args, action.args...)
		cmd = s.cli.NewDockerComposeCmd(s.t, args...)
	case kindDocker:
		cmd = s.cli.NewDockerCmd(s.t, action.args...)
	}
	cmd.Env = append(cmd.Env, s.env...)
	cmd.Env = append(cmd.Env, action.env...)
	return cmd
}

// snapshot captures the current state of the project's containers.
func (s *Scenario) snapshot() snapshot {
	s.t.Helper()
	snap := snapshot{}
	ids := s.projectContainerIDs()
	if len(ids) == 0 {
		return snap
	}
	res := icmd.RunCmd(s.cli.NewDockerCmd(s.t, append([]string{"inspect"}, ids...)...))
	if res.ExitCode != 0 {
		return snap
	}
	var containers []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(res.Stdout()), &containers); err != nil {
		return snap
	}
	for _, c := range containers {
		service := c.Config.Labels["com.docker.compose.service"]
		snap[service] = append(snap[service], containerState{
			ID:     c.ID,
			Name:   strings.TrimPrefix(c.Name, "/"),
			Labels: c.Config.Labels,
		})
	}
	for service := range snap {
		slices.SortFunc(snap[service], func(a, b containerState) int { return strings.Compare(a.Name, b.Name) })
	}
	return snap
}

func (s *Scenario) projectContainerIDs() []string {
	res := icmd.RunCmd(s.cli.NewDockerCmd(s.t, "ps", "-a", "--no-trunc",
		"--filter", "label=com.docker.compose.project="+s.project, "--format", "{{.ID}}"))
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout()) == "" {
		return nil
	}
	return Lines(res.Stdout())
}

// fail reports the scenario failure: intent, step transcript, output of the
// failing command, then live diagnostics (project state, engine events since
// the scenario started, container logs).
func (s *Scenario) fail(reason error) {
	t := s.t
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "scenario failed: %s\n", s.intent)
	fmt.Fprintf(&b, "project: %s\n\ntranscript:\n", s.project)
	for i, step := range s.steps {
		mark := "✓"
		if i == len(s.steps)-1 {
			mark = "✗"
		}
		fmt.Fprintf(&b, "  %s %s — %s (exit %d, %s)\n", mark, step.name, step.command, step.result.ExitCode, step.duration.Round(time.Millisecond))
	}
	last := s.steps[len(s.steps)-1]
	fmt.Fprintf(&b, "\nfailure: %v\n\n--- output of failing step\n%s\n", reason, truncate(last.result.Combined(), 4000))

	fmt.Fprintf(&b, "\n--- project containers\n%s\n", truncate(s.diag("ps", "-a", "--no-trunc", "--filter", "label=com.docker.compose.project="+s.project), 2000))
	fmt.Fprintf(&b, "\n--- engine events since scenario start\n%s\n", truncate(s.diag("events",
		"--since", s.start.Format(time.RFC3339Nano), "--until", time.Now().Format(time.RFC3339Nano),
		"--filter", "label=com.docker.compose.project="+s.project), 2000))
	for _, containers := range s.snapshot() {
		for _, c := range containers {
			fmt.Fprintf(&b, "\n--- logs %s\n%s\n", c.Name, truncate(s.diag("logs", "--tail", "30", c.ID), 2000))
		}
	}
	t.Fatal(b.String())
}

func (s *Scenario) diag(args ...string) string {
	res := icmd.RunCmd(s.cli.NewDockerCmd(s.t, args...))
	return strings.TrimSpace(res.Combined())
}

func truncate(out string, limit int) string {
	if len(out) <= limit {
		return out
	}
	return out[:limit] + fmt.Sprintf("\n… (%d more bytes)", len(out)-limit)
}

// ---------- Actions ----------

type actionKind int

const (
	kindCompose actionKind = iota
	kindDocker
)

// Action is a command a step executes: a compose command run against the
// scenario's project, or a raw docker command.
type Action struct {
	kind    actionKind
	args    []string
	env     []string
	mayFail bool
}

// ComposeCmd runs `docker compose <args>` against the scenario's compose
// file and project name.
func ComposeCmd(args ...string) Action {
	return Action{kind: kindCompose, args: args}
}

// DockerCmd runs a raw `docker <args>` command.
func DockerCmd(args ...string) Action {
	return Action{kind: kindDocker, args: args}
}

// WithEnv adds environment variables to this action only.
func (a Action) WithEnv(kv ...string) Action {
	a.env = append(slices.Clone(a.env), kv...)
	return a
}

// MayFail marks the action as best-effort: a non-zero exit does not fail the
// scenario (e.g. removing an image that may not exist).
func (a Action) MayFail() Action {
	a.mayFail = true
	return a
}

// ---------- Requirements ----------

// Requirement checks an environment prerequisite; it returns a non-empty
// skip reason when the requirement is not met.
type Requirement func(t testing.TB, c *CLI) string

// ContainerdImageStore requires the daemon to use the containerd image store.
func ContainerdImageStore(t testing.TB, c *CLI) string {
	t.Helper()
	res := c.RunDockerCmd(t, "info", "-f", "{{json .DriverStatus}}")
	if !strings.Contains(res.Stdout(), "io.containerd.snapshotter.v1") {
		return "daemon is not using the containerd image store"
	}
	return ""
}

// EngineVersionAtLeast requires a minimum daemon major version.
func EngineVersionAtLeast(major int) Requirement {
	return func(t testing.TB, c *CLI) string {
		t.Helper()
		version := c.RunDockerCmd(t, "version", "-f", "{{.Server.Version}}").Combined()
		before, _, _ := strings.Cut(strings.TrimSpace(version), ".")
		if v, err := strconv.Atoi(before); err == nil && v < major {
			return fmt.Sprintf("engine version %s < %d", strings.TrimSpace(version), major)
		}
		return ""
	}
}

// ---------- Checks ----------

// CheckContext gives a check access to the step's result and to the project
// state captured before and after the step.
type CheckContext struct {
	scenario *Scenario
	result   *icmd.Result
	prev     snapshot
	curr     snapshot
}

// Check is a named observable expected to hold after a step.
type Check struct {
	name string
	fn   func(*CheckContext) error
}

// OutputContains expects the command output to contain a string. Prefer
// state-based checks; use this when the CLI's reported decision is itself
// the observable.
func OutputContains(sub string) Check {
	return Check{
		name: fmt.Sprintf("output contains %q", sub),
		fn: func(ctx *CheckContext) error {
			if !strings.Contains(ctx.result.Combined(), sub) {
				return fmt.Errorf("not found in output")
			}
			return nil
		},
	}
}

// OutputNotContains expects the command output not to contain a string.
func OutputNotContains(sub string) Check {
	return Check{
		name: fmt.Sprintf("output does not contain %q", sub),
		fn: func(ctx *CheckContext) error {
			if strings.Contains(ctx.result.Combined(), sub) {
				return fmt.Errorf("found in output")
			}
			return nil
		},
	}
}

// NotRecreated expects the services' containers to be exactly the ones that
// existed before the step (same container IDs).
func NotRecreated(services ...string) Check {
	return Check{
		name: fmt.Sprintf("services %s not recreated", strings.Join(services, ", ")),
		fn: func(ctx *CheckContext) error {
			for _, service := range services {
				before, after := containerIDs(ctx.prev[service]), containerIDs(ctx.curr[service])
				if len(before) == 0 {
					return fmt.Errorf("service %q had no container before the step", service)
				}
				if !slices.Equal(before, after) {
					return fmt.Errorf("service %q containers changed: %v -> %v", service, before, after)
				}
			}
			return nil
		},
	}
}

func containerIDs(containers []containerState) []string {
	ids := make([]string, 0, len(containers))
	for _, c := range containers {
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	return ids
}

// LabelSet expects every container of the service to carry a non-empty label.
func LabelSet(service, key string) Check {
	return Check{
		name: fmt.Sprintf("service %q has label %q set", service, key),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr[service]
			if len(containers) == 0 {
				return fmt.Errorf("service has no container")
			}
			for _, c := range containers {
				if c.Labels[key] == "" {
					return fmt.Errorf("label empty on container %s", c.Name)
				}
			}
			return nil
		},
	}
}

// LabelsDistinct expects the given services to carry pairwise-distinct values
// for a label.
func LabelsDistinct(key string, services ...string) Check {
	return Check{
		name: fmt.Sprintf("services %s have distinct %q labels", strings.Join(services, ", "), key),
		fn: func(ctx *CheckContext) error {
			seen := map[string]string{}
			for _, service := range services {
				containers := ctx.curr[service]
				if len(containers) == 0 {
					return fmt.Errorf("service %q has no container", service)
				}
				value := containers[0].Labels[key]
				if other, dup := seen[value]; dup {
					return fmt.Errorf("services %q and %q share label value %q", other, service, value)
				}
				seen[value] = service
			}
			return nil
		},
	}
}

// LabelUnchanged expects a service's label to have the same value as before
// the step.
func LabelUnchanged(service, key string) Check {
	return Check{
		name: fmt.Sprintf("service %q label %q unchanged", service, key),
		fn: func(ctx *CheckContext) error {
			before, after := ctx.prev[service], ctx.curr[service]
			if len(before) == 0 || len(after) == 0 {
				return fmt.Errorf("service has no container to compare")
			}
			if before[0].Labels[key] != after[0].Labels[key] {
				return fmt.Errorf("label changed: %q -> %q", before[0].Labels[key], after[0].Labels[key])
			}
			return nil
		},
	}
}

// RunsOnPlatform expects the service's container to have been created for
// the given platform (from its image manifest descriptor).
func RunsOnPlatform(service, platform string) Check {
	return Check{
		name: fmt.Sprintf("service %q container created for platform %s", service, platform),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr[service]
			if len(containers) == 0 {
				return fmt.Errorf("service has no container")
			}
			res := icmd.RunCmd(ctx.scenario.cli.NewDockerCmd(ctx.scenario.t, "inspect", "--format",
				"{{.ImageManifestDescriptor.Platform.OS}}/{{.ImageManifestDescriptor.Platform.Architecture}}",
				containers[0].ID))
			if res.ExitCode != 0 {
				return fmt.Errorf("inspect failed: %s", res.Combined())
			}
			if actual := strings.TrimSpace(res.Stdout()); actual != platform {
				return fmt.Errorf("container platform is %s", actual)
			}
			return nil
		},
	}
}
