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

// This file is the scenario vocabulary: the complete set of observables a
// step can expect. It is meant to be read — and reviewed — as a whole; see
// SCENARIO.md for the rules governing its growth. In short: checks observe
// real state (containers, labels, image manifests), are generic (no
// test-specific logic), and are named after the observable they assert.

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

// CheckContext gives a check access to the step's result and to the project
// state captured before and after the step.
type CheckContext struct {
	scenario *Scenario
	result   *icmd.Result
	prev     snapshot
	curr     snapshot
}

// refresh re-observes the project state, so subsequent checks of the same
// step see the latest state rather than the one captured right after the
// command returned.
func (ctx *CheckContext) refresh() {
	s := ctx.scenario
	ctx.curr = s.snapshot()
	s.snaps[len(s.snaps)-1] = ctx.curr
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

// StdoutContains expects the command's stdout to contain a string. Use it
// instead of OutputContains when the expected text could collide with
// progress noise on stderr (e.g. asserting a container's printed output).
func StdoutContains(sub string) Check {
	return Check{
		name: fmt.Sprintf("stdout contains %q", sub),
		fn: func(ctx *CheckContext) error {
			if !strings.Contains(ctx.result.Stdout(), sub) {
				return fmt.Errorf("not found in stdout")
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

// ExitCode expects the step's command to have exited with the given code.
// Only meaningful on a MayFail action: without MayFail, any non-zero exit
// already fails the step before checks run.
func ExitCode(code int) Check {
	return Check{
		name: fmt.Sprintf("command exits with code %d", code),
		fn: func(ctx *CheckContext) error {
			if ctx.result.ExitCode != code {
				return fmt.Errorf("exit code is %d", ctx.result.ExitCode)
			}
			return nil
		},
	}
}

// Eventually retries a state-based check until it holds or the timeout
// expires, re-observing the project state between attempts. Output-based
// checks are not meaningful here: the step's output never changes.
// Polling is delegated to gotest.tools/v3/poll, the same engine the rest of
// the e2e framework uses.
func Eventually(check Check, timeout time.Duration) Check {
	return Check{
		name: fmt.Sprintf("%s within %s", check.name, timeout),
		fn: func(ctx *CheckContext) error {
			first := true
			capture := &pollCapture{}
			done := make(chan struct{})
			// poll.WaitOn reports timeout through TestingT.Fatalf and relies
			// on it halting execution; run it in a goroutine so pollCapture
			// can Goexit and hand the error back to the scenario report
			// instead of failing the test on the spot.
			go func() {
				defer close(done)
				poll.WaitOn(capture, func(poll.LogT) poll.Result {
					if !first {
						ctx.refresh()
					}
					first = false
					if err := check.fn(ctx); err != nil {
						return poll.Continue("%v", err)
					}
					return poll.Success()
				}, poll.WithDelay(500*time.Millisecond), poll.WithTimeout(timeout))
			}()
			<-done
			return capture.err
		},
	}
}

// pollCapture is a poll.TestingT that records the failure instead of failing
// the test, so Eventually can feed it to the scenario failure report.
type pollCapture struct {
	err error
}

func (c *pollCapture) Log(args ...any)                 {}
func (c *pollCapture) Logf(format string, args ...any) {}

func (c *pollCapture) Fatalf(format string, args ...any) {
	c.err = fmt.Errorf(format, args...)
	runtime.Goexit()
}

// ServiceState expects every long-lived container of the service to be in
// the given state (running, exited, restarting, …). One-off (run) containers
// are not considered; see OneOffState.
func ServiceState(service, state string) Check {
	return Check{
		name: fmt.Sprintf("service %q is %s", service, state),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr.service(service)
			if len(containers) == 0 {
				return fmt.Errorf("service has no container")
			}
			for _, c := range containers {
				if c.State != state {
					return fmt.Errorf("container %s is %s", c.Name, c.State)
				}
			}
			return nil
		},
	}
}

// ServiceScale expects the service to have exactly n long-lived containers.
func ServiceScale(service string, n int) Check {
	return Check{
		name: fmt.Sprintf("service %q has %d containers", service, n),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr.service(service)
			if len(containers) != n {
				var names []string
				for _, c := range containers {
					names = append(names, c.Name)
				}
				return fmt.Errorf("found %d: %s", len(names), strings.Join(names, ", "))
			}
			return nil
		},
	}
}

// ReplicaNumbers expects the service's containers to carry exactly the given
// replica numbers (the com.docker.compose.container-number label), locking
// which replicas survive a scale up or down.
func ReplicaNumbers(service string, numbers ...int) Check {
	return Check{
		name: fmt.Sprintf("service %q has replicas %v", service, numbers),
		fn: func(ctx *CheckContext) error {
			var actual []string
			for _, c := range ctx.curr.service(service) {
				actual = append(actual, c.Labels["com.docker.compose.container-number"])
			}
			slices.Sort(actual)
			var expected []string
			for _, n := range numbers {
				expected = append(expected, fmt.Sprint(n))
			}
			slices.Sort(expected)
			if !slices.Equal(actual, expected) {
				return fmt.Errorf("found replicas %v", actual)
			}
			return nil
		},
	}
}

// ServiceNotCreated expects the service to have no container at all — one-off
// containers included — e.g. after an action that must leave unrelated
// services untouched.
func ServiceNotCreated(service string) Check {
	return Check{
		name: fmt.Sprintf("service %q has no container", service),
		fn: func(ctx *CheckContext) error {
			var names []string
			for _, c := range ctx.curr[service] {
				names = append(names, c.Name)
			}
			if len(names) > 0 {
				return fmt.Errorf("found %s", strings.Join(names, ", "))
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
				before, after := containerIDs(ctx.prev.service(service)), containerIDs(ctx.curr.service(service))
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

// Recreated expects the services' containers to have been replaced by the
// step: none of the containers that existed before survived it.
func Recreated(services ...string) Check {
	return Check{
		name: fmt.Sprintf("services %s recreated", strings.Join(services, ", ")),
		fn: func(ctx *CheckContext) error {
			for _, service := range services {
				before, after := containerIDs(ctx.prev.service(service)), containerIDs(ctx.curr.service(service))
				if len(before) == 0 {
					return fmt.Errorf("service %q had no container before the step", service)
				}
				for _, id := range after {
					if slices.Contains(before, id) {
						return fmt.Errorf("service %q kept container %s", service, id[:12])
					}
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

// ServiceHealthy expects every container of the service to report a healthy
// state from its healthcheck.
func ServiceHealthy(service string) Check {
	return Check{
		name: fmt.Sprintf("service %q is healthy", service),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr.service(service)
			if len(containers) == 0 {
				return fmt.Errorf("service has no container")
			}
			for _, c := range containers {
				res := icmd.RunCmd(ctx.scenario.cli.NewDockerCmd(ctx.scenario.t,
					"inspect", "--format", "{{.State.Health.Status}}", c.ID))
				if res.ExitCode != 0 {
					return fmt.Errorf("inspect failed: %s", res.Combined())
				}
				if status := strings.TrimSpace(res.Stdout()); status != "healthy" {
					return fmt.Errorf("container %s is %s", c.Name, status)
				}
			}
			return nil
		},
	}
}

// OneOffState expects every one-off (run) container of the service to be in
// the given state.
func OneOffState(service, state string) Check {
	return Check{
		name: fmt.Sprintf("one-off of service %q is %s", service, state),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr.oneOffs(service)
			if len(containers) == 0 {
				return fmt.Errorf("service has no one-off container")
			}
			for _, c := range containers {
				if c.State != state {
					return fmt.Errorf("container %s is %s", c.Name, c.State)
				}
			}
			return nil
		},
	}
}

// OneOffsUntouched expects the service's one-off containers to be exactly the
// ones from before the step, neither restarted nor removed: same container
// IDs, same state, same start time.
func OneOffsUntouched(service string) Check {
	return Check{
		name: fmt.Sprintf("one-offs of service %q untouched", service),
		fn: func(ctx *CheckContext) error {
			before, after := ctx.prev.oneOffs(service), ctx.curr.oneOffs(service)
			if len(before) == 0 {
				return fmt.Errorf("service had no one-off container before the step")
			}
			if !slices.Equal(containerIDs(before), containerIDs(after)) {
				return fmt.Errorf("one-off containers changed: %v -> %v", containerIDs(before), containerIDs(after))
			}
			for i, b := range before {
				a := after[i]
				if a.State != b.State || a.StartedAt != b.StartedAt {
					return fmt.Errorf("container %s was touched: %s (started %s) -> %s (started %s)",
						b.Name, b.State, b.StartedAt, a.State, a.StartedAt)
				}
			}
			return nil
		},
	}
}

// OneOffsRemoved expects the one-off containers the service had before the
// step to be gone; it errors if the service had none, rather than pass
// vacuously.
func OneOffsRemoved(service string) Check {
	return Check{
		name: fmt.Sprintf("one-offs of service %q removed", service),
		fn: func(ctx *CheckContext) error {
			if len(ctx.prev.oneOffs(service)) == 0 {
				return fmt.Errorf("service had no one-off container before the step")
			}
			var names []string
			for _, c := range ctx.curr.oneOffs(service) {
				names = append(names, c.Name)
			}
			if len(names) > 0 {
				return fmt.Errorf("still present: %s", strings.Join(names, ", "))
			}
			return nil
		},
	}
}

// ImageExists expects an image with the given reference to be present in the
// local store.
func ImageExists(ref string) Check {
	return Check{
		name: fmt.Sprintf("image %q exists", ref),
		fn: func(ctx *CheckContext) error {
			res := icmd.RunCmd(ctx.scenario.cli.NewDockerCmd(ctx.scenario.t, "image", "inspect", "--format", "{{.Id}}", ref))
			if res.ExitCode != 0 {
				return fmt.Errorf("not found: %s", strings.TrimSpace(res.Combined()))
			}
			return nil
		},
	}
}

// FileExists expects a non-empty file at the given host path, e.g. the output
// of an export command.
func FileExists(path string) Check {
	return Check{
		name: fmt.Sprintf("file %q exists", path),
		fn: func(ctx *CheckContext) error {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				return fmt.Errorf("file is empty")
			}
			return nil
		},
	}
}

// FileContains expects the host file at the given path to contain a string,
// e.g. a file copied out of a container.
func FileContains(path, sub string) Check {
	return Check{
		name: fmt.Sprintf("file %q contains %q", path, sub),
		fn: func(ctx *CheckContext) error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), sub) {
				return fmt.Errorf("not found in file content: %q", truncate(string(data), 200))
			}
			return nil
		},
	}
}

// LabelSet expects every container of the service to carry a non-empty label.
func LabelSet(service, key string) Check {
	return Check{
		name: fmt.Sprintf("service %q has label %q set", service, key),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr.service(service)
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
				containers := ctx.curr.service(service)
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
			before, after := ctx.prev.service(service), ctx.curr.service(service)
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
			containers := ctx.curr.service(service)
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
