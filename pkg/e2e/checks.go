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

// ServiceState expects every container of the service to be in the given
// state (running, exited, restarting, …).
func ServiceState(service, state string) Check {
	return Check{
		name: fmt.Sprintf("service %q is %s", service, state),
		fn: func(ctx *CheckContext) error {
			containers := ctx.curr[service]
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
