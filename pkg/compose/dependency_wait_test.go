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

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestWaitDependencies(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	t.Run("should skip dependencies with scale 0", func(t *testing.T) {
		dbService := types.ServiceConfig{Name: "db", Scale: intPtr(0)}
		redisService := types.ServiceConfig{Name: "redis", Scale: intPtr(0)}
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db":    dbService,
			"redis": redisService,
		}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: ServiceConditionRunningOrHealthy},
			"redis": {Condition: ServiceConditionRunningOrHealthy},
		}
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "", dependencies, nil))
	})
	t.Run("should skip zero-replica dependencies after service hashing", func(t *testing.T) {
		replicas := 0
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"app": {
				Name: "app", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
					"disabled": {
						Condition: ServiceConditionRunningOrHealthy,
						Required:  true,
					},
				}},
			},
			"disabled": {
				Name:   "disabled",
				Deploy: &types.DeployConfig{Replicas: &replicas},
			},
		}}

		_, err := ServiceHash(project.Services["disabled"])
		assert.NilError(t, err)

		assert.NilError(t, tested.(*composeService).waitDependencies(
			t.Context(), &project, "app", project.Services["app"].DependsOn, nil,
		))
	})
	t.Run("should skip dependencies with condition service_started", func(t *testing.T) {
		dbService := types.ServiceConfig{Name: "db", Scale: intPtr(1)}
		redisService := types.ServiceConfig{Name: "redis", Scale: intPtr(1)}
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db":    dbService,
			"redis": redisService,
		}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: types.ServiceConditionStarted, Required: true},
			"redis": {Condition: types.ServiceConditionStarted, Required: true},
		}
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "", dependencies, nil))
	})
	t.Run("missing required dependency is an error", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: true},
		}
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, nil)
		assert.Error(t, err, "app is missing dependency db")
	})
	t.Run("a dependency's pre_start hook runner does not count as the dependency being present", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1), PreStart: []types.PreStartHook{{}}},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: true},
		}
		// Only a retained hook runner exists (e.g. its own success removal is
		// still in flight, or it failed and was kept for inspection) — no
		// real "db" replica container.
		containers := Containers{{
			ID:     "db-hook-runner",
			Names:  []string{"/db-hook-runner"},
			Labels: map[string]string{api.ServiceLabel: "db", api.HookLabel: "pre_start"},
		}}
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers)
		assert.Error(t, err, "app is missing dependency db")
	})
	t.Run("missing optional dependency is only a warning", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: false},
		}
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, nil))
	})
	t.Run("failing optional dependency is skipped, not an error", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: types.ServiceConditionHealthy, Required: false},
		}
		containers := Containers{{
			ID:     "db-ctr",
			Names:  []string{"/db-ctr"},
			Labels: map[string]string{api.ServiceLabel: "db"},
		}}
		// The dependency exited: a required dependency would fail the wait,
		// an optional one is skipped after the first poll.
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    "db-ctr",
				Name:  "/db-ctr",
				State: &container.State{Status: container.StateExited, ExitCode: 1},
			},
		}, nil)
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers))
	})
	t.Run("dry-run completes service_completed_successfully immediately", func(t *testing.T) {
		tested.(*composeService).dryRun = true

		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"init": {Name: "init", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"init": {Condition: types.ServiceConditionCompletedSuccessfully, Required: true},
		}
		containers := Containers{{
			ID:     "init-ctr",
			Names:  []string{"/init-ctr"},
			Labels: map[string]string{api.ServiceLabel: "init"},
		}}
		// no ContainerInspect expectation: dry-run must not inspect a
		// container that was never created.
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers))
	})
}

func TestIsServiceHealthy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	ctx := t.Context()

	t.Run("disabled healthcheck with fallback to running", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck (Test: ["NONE"])
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with disabled healthcheck should be considered healthy when running with fallbackRunning=true")
	})

	t.Run("disabled healthcheck without fallback", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck (Test: ["NONE"]) but fallbackRunning=false
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		_, err := tested.(*composeService).isServiceHealthy(ctx, containers, false)
		assert.ErrorContains(t, err, "has no healthcheck configured")
	})

	t.Run("no healthcheck with fallback to running", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with no healthcheck at all
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: nil,
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with no healthcheck should be considered healthy when running with fallbackRunning=true")
	})

	t.Run("exited container with disabled healthcheck", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck but exited
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:   containerID,
				Name: "test-container",
				State: &container.State{
					Status:   "exited",
					ExitCode: 1,
				},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		_, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.ErrorContains(t, err, "exited")
	})

	t.Run("healthy container with healthcheck", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with actual healthcheck that is healthy
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:   containerID,
				Name: "test-container",
				State: &container.State{
					Status: "running",
					Health: &container.Health{
						Status: container.Healthy,
					},
				},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"CMD", "curl", "-f", "http://localhost"},
					},
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, false)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with healthy status should be healthy")
	})
}

// startingDBInspect is db-ctr running with its healthcheck still starting.
var startingDBInspect = client.ContainerInspectResult{
	Container: container.InspectResponse{
		ID:     "db-ctr",
		Name:   "/db-ctr",
		Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		State:  &container.State{Status: container.StateRunning, Health: &container.Health{Status: container.Starting}},
	},
}

// waitWithOwnTimeout runs one depends_on wait the way the plan's
// OpWaitCondition nodes do under --wait-timeout: bounded by a timeout of its
// own (planExecutor.waitTimeout), the only way a wait gets one. A watchdog
// ends a wait that would ignore that timeout.
func waitWithOwnTimeout(t *testing.T, svc *composeService, project *types.Project, containers Containers, condition string, required bool, timeout time.Duration) error {
	t.Helper()
	exec := svc.newPlanExecutor(project, emptyObservedState(project.Name))
	exec.containersByService["db"] = containers
	exec.waitTimeout = timeout
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	return exec.execBoundedWaitCondition(ctx, Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:" + condition,
		Name:       "db",
		Condition:  condition,
		BestEffort: !required,
	})
}

// TestWaitDependencyDeadline locks the timeout semantics of the dependency
// wait: an expired deadline surfaces as "timeout waiting for dependencies",
// while a plain user cancellation is not a wait failure.
func TestWaitDependencyDeadline(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
		"db": {Name: "db", Scale: intPtr(1)},
	}}
	dependencies := types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionHealthy, Required: true},
	}
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	// stillStarting lets the timeout below expire between two checks.
	stillStarting := func() {
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).Return(startingDBInspect, nil)
	}

	t.Run("expired deadline is an error", func(t *testing.T) {
		// The caller's deadline (--wait-timeout of up/start --wait) fires
		// between two checks and must not be swallowed.
		stillStarting()
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		err := tested.(*composeService).waitDependencies(ctx, &project, "app", dependencies, containers)
		assert.Error(t, err, "timeout waiting for dependencies")
	})

	t.Run("expired own timeout of a plan wait is an error", func(t *testing.T) {
		// The same, for the timeout of the plan's own wait node.
		stillStarting()
		err := waitWithOwnTimeout(t, tested.(*composeService), &project, containers, types.ServiceConditionHealthy, true, 50*time.Millisecond)
		assert.Assert(t, errors.Is(err, errWaitTimeout), "got %v", err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	// Regression guard: an optional dependency's deadline expiring (the
	// timeout of the plan's own wait node, planExecutor.waitTimeout -- see
	// waitDependency's ctx.Done() branch) must be tolerated exactly like any
	// other definitive failure of an optional dependency, not surfaced as
	// "timeout waiting for dependencies".
	t.Run("expired own timeout on an optional dependency is skipped, not an error", func(t *testing.T) {
		stillStarting()
		err := waitWithOwnTimeout(t, tested.(*composeService), &project, containers, types.ServiceConditionHealthy, false, 50*time.Millisecond)
		assert.NilError(t, err)
	})

	// Regression guard: the tolerance above must apply only to a deadline
	// THIS wait's own timeout is responsible for -- an unrelated,
	// already-expired deadline the caller brought in (waitDependencies sets
	// none of its own) is a different, real failure that must still
	// propagate even for an optional dependency.
	t.Run("an inherited parent deadline is not swallowed for an optional dependency", func(t *testing.T) {
		optionalDeps := types.DependsOnConfig{
			"db": {Condition: types.ServiceConditionHealthy, Required: false},
		}
		parentCtx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // let the parent's own deadline actually expire first
		err := tested.(*composeService).waitDependencies(parentCtx, &project, "app", optionalDeps, containers)
		assert.Error(t, err, "timeout waiting for dependencies")
	})

	t.Run("user cancellation is not a wait failure", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := tested.(*composeService).waitDependencies(ctx, &project, "app", dependencies, containers)
		assert.NilError(t, err)
	})

	// Regression guard for the runtime fallback warning on waitDependency's
	// default branch — see the comment on that branch for why it must stay.
	t.Run("unsupported condition warns and returns without waiting", func(t *testing.T) {
		hook := logrustest.NewGlobal()
		unsupportedDeps := types.DependsOnConfig{
			"db": {Condition: "some_future_condition", Required: true},
		}
		// the bound makes a regression that waits fail instead of hanging
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		err := tested.(*composeService).waitDependencies(ctx, &project, "app", unsupportedDeps, containers)
		assert.NilError(t, err)

		var messages []string
		for _, e := range hook.AllEntries() {
			messages = append(messages, e.Message)
		}
		joined := strings.Join(messages, "\n")
		assert.Assert(t, strings.Contains(joined, `service "app": unsupported depends_on condition "some_future_condition"`), joined)
	})
}

// TestWaitDependencyFirstCheck locks when a satisfied dependency is first
// checked: on entry, except running_or_healthy (see waitDependencyReached).
func TestWaitDependencyFirstCheck(t *testing.T) {
	project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
		"db": {Name: "db", Scale: intPtr(1)},
	}}
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	satisfied := map[string]struct {
		inspect container.InspectResponse
		wait    time.Duration
	}{
		types.ServiceConditionHealthy: {
			inspect: container.InspectResponse{
				ID:     "db-ctr",
				Name:   "/db-ctr",
				Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
				State:  &container.State{Status: container.StateRunning, Health: &container.Health{Status: container.Healthy}},
			},
			wait: 0,
		},
		types.ServiceConditionCompletedSuccessfully: {
			inspect: container.InspectResponse{
				ID:    "db-ctr",
				Name:  "/db-ctr",
				State: &container.State{Status: container.StateExited, ExitCode: 0},
			},
			wait: 0,
		},
		ServiceConditionRunningOrHealthy: {
			inspect: container.InspectResponse{
				ID:     "db-ctr",
				Name:   "/db-ctr",
				Config: &container.Config{},
				State:  &container.State{Status: container.StateRunning},
			},
			wait: 500 * time.Millisecond,
		},
	}

	for condition, tc := range satisfied {
		t.Run(condition, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				apiClient := mocks.NewMockAPIClient(mockCtrl)
				cli := mocks.NewMockCli(mockCtrl)
				cli.EXPECT().Client().Return(apiClient).AnyTimes()
				tested, err := NewComposeService(cli)
				assert.NilError(t, err)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{Container: tc.inspect}, nil)

				dependencies := types.DependsOnConfig{"db": {Condition: condition, Required: true}}
				start := time.Now()
				err = tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers)
				assert.NilError(t, err)
				assert.Equal(t, time.Since(start), tc.wait)
			})
		})
	}
}

// TestOptionalDependencyInspectFailure locks what an optional (required:
// false) dependency's failing inspection means. A genuine inspection error
// (the daemon doesn't know the container, the API call fails) says the
// dependency didn't start, so it is skipped. A failure caused by the wait's
// context being done says nothing about the dependency: it must not be
// turned into a skipped dependency, i.e. into a successful wait, and the end
// of the wait is left to waitDependency (a deadline is an error, a plain
// cancellation stays silent). A required dependency is never skipped.
func TestOptionalDependencyInspectFailure(t *testing.T) {
	const skipped = "Skipped: "
	conditions := []string{ServiceConditionRunningOrHealthy, types.ServiceConditionHealthy}
	dbContainers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
		"db": {Name: "db", Scale: intPtr(1)},
	}}

	newTested := func(t *testing.T) (*composeService, *mocks.MockAPIClient, *capturingEvents) {
		t.Helper()
		mockCtrl := gomock.NewController(t)
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		events := &capturingEvents{}
		tested, err := NewComposeService(cli, WithEventProcessor(events))
		assert.NilError(t, err)
		return tested.(*composeService), apiClient, events
	}
	wasSkipped := func(events *capturingEvents) bool {
		for _, r := range events.resources {
			if strings.HasPrefix(r.Text, skipped) {
				return true
			}
		}
		return false
	}
	dependenciesOn := func(condition string, required bool) types.DependsOnConfig {
		return types.DependsOnConfig{"db": {Condition: condition, Required: required}}
	}
	// inspectFailsWithContext makes ContainerInspect fail the way the client
	// does when the context it was given is done: it waits for it and
	// returns its error, wrapped. It must be reached at least once (or the
	// test would pass without exercising the inspection); once the context is
	// done, a pending poll tick may win the select against ctx.Done() for one
	// more check, so there is no upper bound.
	inspectFailsWithContext := func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				<-ctx.Done()
				return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", ctx.Err())
			}).MinTimes(1)
	}

	for _, condition := range conditions {
		t.Run(condition, func(t *testing.T) {
			t.Run("a genuine inspection error skips an optional dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, errors.New("Error response from daemon: No such container: db-ctr"))
				err := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, false), dbContainers)
				assert.NilError(t, err)
				assert.Assert(t, wasSkipped(events))
			})

			t.Run("a genuine inspection error fails a required dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, errors.New("Error response from daemon: No such container: db-ctr"))
				err := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, true), dbContainers)
				assert.ErrorContains(t, err, "No such container: db-ctr")
				assert.Assert(t, !wasSkipped(events))
			})

			t.Run("a cancellation during the inspection is not a skipped optional dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
						cancel()
						return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", context.Canceled)
					}).MinTimes(1) // a pending poll tick may win the select once more
				// a plain cancellation is not a wait failure (see
				// TestWaitDependencyDeadline), but it is no skip either
				err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, false), dbContainers)
				assert.NilError(t, err)
				assert.Assert(t, !wasSkipped(events))
			})

			t.Run("a cancellation during the inspection still fails a required dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, _ := newTested(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
						cancel()
						return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", context.Canceled)
					})
				err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, true), dbContainers)
				assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
			})

			for _, required := range []bool{false, true} {
				t.Run(fmt.Sprintf("a deadline inherited from the caller expiring during the inspection is an error (required=%t)", required), func(t *testing.T) {
					t.Parallel()
					tested, apiClient, events := newTested(t)
					inspectFailsWithContext(apiClient)
					// the first check starts the inspection, which only ends
					// with the deadline
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, required), dbContainers)
					assert.Error(t, err, "timeout waiting for dependencies")
					assert.Assert(t, !wasSkipped(events))
				})
			}

			// The wait's own timeout expiring during the inspection must end
			// the wait exactly as it does when it expires between two polls:
			// whatever waitDependency decides for an optional dependency, the
			// check functions must not decide differently.
			for _, required := range []bool{false, true} {
				t.Run(fmt.Sprintf("the wait's own timeout expiring during the inspection ends like it does between polls (required=%t)", required), func(t *testing.T) {
					t.Parallel()
					tested, apiClient, _ := newTested(t)
					inspectFailsWithContext(apiClient)
					duringInspect := waitWithOwnTimeout(t, tested, &project, dbContainers, condition, required, time.Second)

					// no check in flight when the timeout expires: at most the one on entry
					polled, polledClient, _ := newTested(t)
					polledClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).Return(startingDBInspect, nil).MaxTimes(1)
					betweenPolls := waitWithOwnTimeout(t, polled, &project, dbContainers, condition, required, 50*time.Millisecond)

					// the same end, whatever the instant the timeout expired at
					assert.Equal(t, duringInspect == nil, betweenPolls == nil)
					assert.Equal(t, errors.Is(duringInspect, errWaitTimeout), errors.Is(betweenPolls, errWaitTimeout))
					assert.Equal(t, errors.Is(duringInspect, context.DeadlineExceeded), errors.Is(betweenPolls, context.DeadlineExceeded))
					if required {
						assert.Assert(t, errors.Is(duringInspect, errWaitTimeout), "got %v", duringInspect)
						assert.ErrorIs(t, duringInspect, context.DeadlineExceeded)
					} else {
						assert.NilError(t, duringInspect)
					}
				})
			}
		})
	}
}

// TestCheckDependencyDoneContext tests the check functions directly with a
// context that is already done, so no poll timing is involved: an optional
// dependency is "not done yet" (the wait's loop then sees ctx.Done()), never
// skipped; a required one keeps reporting the failure.
// A genuine failure of a required dependency is reported even when the context
// happens to be canceled by the time it is evaluated: only an error caused by
// the cancellation is silent.
func TestCheckDependencyHealthyReportsGenuineFailureOnCanceledContext(t *testing.T) {
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	events := &capturingEvents{}
	svc, err := NewComposeService(cli, WithEventProcessor(events))
	assert.NilError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			// the phase is canceled right after the inspection answered
			cancel()
			return client.ContainerInspectResult{
				Container: container.InspectResponse{
					ID:   "db-ctr",
					Name: "/db-ctr",
					State: &container.State{
						Status: container.StateRunning,
						Health: &container.Health{Status: container.Unhealthy},
					},
					Config: &container.Config{
						Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}},
					},
				},
			}, nil
		})

	done, err := svc.(*composeService).checkDependencyHealthy(ctx, "db", types.ServiceDependency{Required: true}, containers)
	assert.ErrorContains(t, err, "is unhealthy")
	assert.Assert(t, !done)
	assert.Equal(t, len(events.resources), 1, "got %v", events.resources)
	assert.Equal(t, events.resources[0].Status, api.Error)
}

func TestCheckDependencyDoneContext(t *testing.T) {
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	expired := func(t *testing.T) context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	canceled := func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	contexts := map[string]func(*testing.T) context.Context{"canceled": canceled, "expired": expired}
	checks := map[string]func(*composeService, context.Context, types.ServiceDependency) (bool, error){
		"running_or_healthy": func(s *composeService, ctx context.Context, c types.ServiceDependency) (bool, error) {
			return s.checkDependencyRunningOrHealthy(ctx, "db", c, containers)
		},
		"service_healthy": func(s *composeService, ctx context.Context, c types.ServiceDependency) (bool, error) {
			return s.checkDependencyHealthy(ctx, "db", c, containers)
		},
	}

	for checkName, check := range checks {
		for ctxName, newCtx := range contexts {
			t.Run(checkName+"/"+ctxName, func(t *testing.T) {
				t.Parallel()
				mockCtrl := gomock.NewController(t)
				apiClient := mocks.NewMockAPIClient(mockCtrl)
				cli := mocks.NewMockCli(mockCtrl)
				cli.EXPECT().Client().Return(apiClient).AnyTimes()
				events := &capturingEvents{}
				svc, err := NewComposeService(cli, WithEventProcessor(events))
				assert.NilError(t, err)
				tested := svc.(*composeService)

				ctx := newCtx(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", ctx.Err())).Times(2)

				done, err := check(tested, ctx, types.ServiceDependency{Required: false})
				assert.NilError(t, err)
				assert.Assert(t, !done, "an optional dependency must not complete on a done context")
				assert.Equal(t, len(events.resources), 0, "no skipped event expected, got %v", events.resources)

				_, err = check(tested, ctx, types.ServiceDependency{Required: true})
				assert.Assert(t, errors.Is(err, ctx.Err()), "got %v", err)
			})
		}
	}

	// service_completed_successfully never skipped an optional dependency on
	// an inspection error (only on a non-zero exit code): unchanged.
	t.Run("service_completed_successfully", func(t *testing.T) {
		t.Parallel()
		mockCtrl := gomock.NewController(t)
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		svc, err := NewComposeService(cli)
		assert.NilError(t, err)

		ctx := canceled(t)
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
			Return(client.ContainerInspectResult{}, ctx.Err())
		done, err := svc.(*composeService).checkDependencyCompleted(ctx, "db", types.ServiceDependency{Required: false}, containers)
		assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
		assert.Assert(t, !done)
	})
}
