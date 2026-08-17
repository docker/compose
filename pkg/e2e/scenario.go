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

	"golang.org/x/tools/txtar"
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
	ID        string
	Name      string
	State     string
	StartedAt string
	OneOff    bool
	Labels    map[string]string
}

// snapshot maps a service name to its containers — long-lived and one-off
// (run) alike — sorted by name.
type snapshot map[string][]containerState

// service returns the service's long-lived containers, one-offs excluded.
func (s snapshot) service(name string) []containerState {
	var containers []containerState
	for _, c := range s[name] {
		if !c.OneOff {
			containers = append(containers, c)
		}
	}
	return containers
}

// oneOffs returns the service's one-off (run) containers.
func (s snapshot) oneOffs(name string) []containerState {
	var containers []containerState
	for _, c := range s[name] {
		if c.OneOff {
			containers = append(containers, c)
		}
	}
	return containers
}

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
		if t.Failed() && os.Getenv("E2E_KEEP_FAILED") != "" {
			t.Logf("E2E_KEEP_FAILED set: keeping project %s alive for inspection (docker ps --filter label=com.docker.compose.project=%s)", s.project, s.project)
			return
		}
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

// Project returns the compose project name the scenario runs under, e.g. to
// Defer the removal of an image the project built.
func (s *Scenario) Project() string { return s.project }

// Dir returns the project directory holding the files declared by Compose or
// Files, for actions that exchange files with the host (e.g. cp). It is only
// valid after the compose model has been declared.
func (s *Scenario) Dir() string {
	s.t.Helper()
	if s.file == "" {
		s.t.Fatal("Dir() called before Compose or Files")
	}
	return filepath.Dir(s.file)
}

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

// Files declares the project's files — compose.yaml plus whatever it needs
// (Dockerfile, .env, config files) — as a txtar archive: each file introduced
// by a `-- name --` line, extracted into the project directory. The archive
// must contain a compose.yaml, which becomes the scenario's compose file.
// txtar is the format Go's own cmd/go tests are written in: diff-friendly,
// and trivial to read and write for humans and coding agents alike.
func (s *Scenario) Files(archive string) *Scenario {
	s.t.Helper()
	dir := s.t.TempDir()
	for _, f := range txtar.Parse([]byte(archive)).Files {
		path := filepath.Join(dir, f.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			s.t.Fatalf("failed to create directory for %s: %v", f.Name, err)
		}
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			s.t.Fatalf("failed to write %s: %v", f.Name, err)
		}
		if f.Name == "compose.yaml" {
			s.file = path
		}
	}
	if s.file == "" {
		s.t.Fatal("Files archive must contain a compose.yaml")
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
	cmd.Timeout = action.timeout
	if action.stdin != "" {
		cmd.Stdin = strings.NewReader(action.stdin)
	}
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
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status    string `json:"Status"`
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
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
			ID:        c.ID,
			Name:      strings.TrimPrefix(c.Name, "/"),
			State:     c.State.Status,
			StartedAt: c.State.StartedAt,
			OneOff:    c.Config.Labels["com.docker.compose.oneoff"] == "True",
			Labels:    c.Config.Labels,
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
// the scenario started, container logs). Inline sections are truncated for
// readability; the full, untruncated material is written to an artifacts
// directory whose path opens the report.
func (s *Scenario) fail(reason error) {
	t := s.t
	t.Helper()

	live := s.snapshot()
	containersOut := s.diag("ps", "-a", "--no-trunc", "--filter", "label=com.docker.compose.project="+s.project)
	eventsOut := s.diag("events",
		"--since", s.start.Format(time.RFC3339Nano), "--until", time.Now().Format(time.RFC3339Nano),
		"--filter", "label=com.docker.compose.project="+s.project)
	artifacts := s.writeArtifacts(reason, live, containersOut, eventsOut)

	var b strings.Builder
	fmt.Fprintf(&b, "scenario failed: %s\n", s.intent)
	fmt.Fprintf(&b, "project: %s\n", s.project)
	if artifacts != "" {
		fmt.Fprintf(&b, "artifacts: %s (compose.yaml, full step outputs, events, logs)\n", artifacts)
	}
	if os.Getenv("E2E_KEEP_FAILED") == "" {
		fmt.Fprintf(&b, "hint: rerun with E2E_KEEP_FAILED=1 to keep the project alive for inspection\n")
	}
	fmt.Fprintf(&b, "\ntranscript:\n")
	for i, step := range s.steps {
		mark := "✓"
		if i == len(s.steps)-1 {
			mark = "✗"
		}
		fmt.Fprintf(&b, "  %s %s — %s (exit %d, %s)\n", mark, step.name, step.command, step.result.ExitCode, step.duration.Round(time.Millisecond))
	}
	last := s.steps[len(s.steps)-1]
	fmt.Fprintf(&b, "\nfailure: %v\n\n--- output of failing step\n%s\n", reason, truncate(last.result.Combined(), 4000))

	fmt.Fprintf(&b, "\n--- project containers\n%s\n", truncate(containersOut, 2000))
	fmt.Fprintf(&b, "\n--- engine events since scenario start\n%s\n", truncate(eventsOut, 2000))
	for _, containers := range live {
		for _, c := range containers {
			fmt.Fprintf(&b, "\n--- logs %s\n%s\n", c.Name, truncate(s.diag("logs", "--tail", "30", c.ID), 2000))
		}
	}
	t.Fatal(b.String())
}

// writeArtifacts dumps the untruncated failure material to a stable directory
// (one per project, overwritten on each run) so a failure can be diagnosed —
// by a human or a coding agent — without re-running the scenario: the compose
// model, each step's full command and output, the project containers, the
// engine events, every container's full logs and the per-step state
// snapshots. Returns the directory path, or "" if it could not be written.
func (s *Scenario) writeArtifacts(reason error, live snapshot, containersOut, eventsOut string) string {
	dir := filepath.Join(os.TempDir(), "compose-e2e-artifacts", s.project)
	if err := os.RemoveAll(dir); err != nil {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	write := func(name, content string) {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}

	if s.file != "" {
		if data, err := os.ReadFile(s.file); err == nil {
			write("compose.yaml", string(data))
		}
	}
	write("failure.txt", fmt.Sprintf("scenario: %s\nproject: %s\nfailure: %v\n", s.intent, s.project, reason))
	for i, step := range s.steps {
		write(fmt.Sprintf("step-%02d-%s.txt", i+1, slugify(step.name)),
			fmt.Sprintf("step: %s\ncommand: %s\nexit code: %d\nduration: %s\n\n%s",
				step.name, step.command, step.result.ExitCode, step.duration.Round(time.Millisecond), step.result.Combined()))
	}
	write("containers.txt", containersOut)
	write("events.txt", eventsOut)
	for _, containers := range live {
		for _, c := range containers {
			write("logs-"+c.Name+".txt", s.diag("logs", c.ID))
		}
	}
	if data, err := json.MarshalIndent(s.snaps, "", "  "); err == nil {
		write("snapshots.json", string(data))
	}
	return dir
}

// slugify turns a free-form step name into a safe file-name fragment.
func slugify(name string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteRune('-')
			}
			pendingDash = false
			b.WriteRune(r)
		} else {
			pendingDash = true
		}
	}
	return b.String()
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
	stdin   string
	mayFail bool
	timeout time.Duration
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

// WithStdin feeds the command's standard input, e.g. to answer an interactive
// prompt ("n\n").
func (a Action) WithStdin(input string) Action {
	a.stdin = input
	return a
}

// Within bounds the command's execution time, for blocking commands whose
// termination is itself the expectation (e.g. `up --wait` on a service that
// must become healthy). Exceeding the timeout kills the command and fails the
// step.
func (a Action) Within(timeout time.Duration) Action {
	a.timeout = timeout
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
