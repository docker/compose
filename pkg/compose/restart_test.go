//go:build !windows

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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// These tests characterize the restart path: which depends_on relations
// survive project preparation, and the per-container sequence (pre_stop →
// ContainerRestart → post_start) with its events. restart still walks the
// dependency order itself (InDependencyOrder) but shares the hook/wait
// primitives with the plan.

// TestPrepareRestartProject locks the selection semantics: only depends_on
// relations declaring restart:true are kept, and naming services includes
// their dependents (never their dependencies).
func TestPrepareRestartProject(t *testing.T) {
	svc, _, _ := newStartTestService(t)

	// A fresh project per subtest: the transforms applied by
	// prepareRestartProject share the DependsOn maps with their input.
	newProject := func() *types.Project {
		return &types.Project{
			Name: "prj",
			Services: types.Services{
				"proxy": {
					Name: "proxy", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
						"web": {Condition: types.ServiceConditionStarted, Restart: true, Required: true},
					}},
				},
				"web": {
					Name: "web", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
						"db": {Condition: types.ServiceConditionStarted, Restart: false, Required: true},
					}},
				},
				"db": {Name: "db"},
			},
		}
	}

	t.Run("only restart:true relations survive", func(t *testing.T) {
		prepared, err := svc.prepareRestartProject(t.Context(), nil, "prj", api.RestartOptions{Project: newProject()})
		assert.NilError(t, err)
		_, hasWebDep := prepared.Services["proxy"].DependsOn["web"]
		assert.Assert(t, hasWebDep)
		_, hasDBDep := prepared.Services["web"].DependsOn["db"]
		assert.Assert(t, !hasDBDep)
	})

	t.Run("naming a service includes its dependents, not its dependencies", func(t *testing.T) {
		prepared, err := svc.prepareRestartProject(t.Context(), nil, "prj", api.RestartOptions{
			Project:  newProject(),
			Services: []string{"web"},
		})
		assert.NilError(t, err)
		names := prepared.ServiceNames()
		assert.Assert(t, len(names) == 2, "got %v", names)
		assert.Assert(t, prepared.Services["web"].Name == "web")
		// proxy depends on web with restart:true → restarted alongside
		assert.Assert(t, prepared.Services["proxy"].Name == "proxy")
	})

	t.Run("no-deps drops the dependents", func(t *testing.T) {
		prepared, err := svc.prepareRestartProject(t.Context(), nil, "prj", api.RestartOptions{
			Project:  newProject(),
			Services: []string{"web"},
			NoDeps:   true,
		})
		assert.NilError(t, err)
		assert.DeepEqual(t, prepared.ServiceNames(), []string{"web"})
	})
}

// TestRestartContainer_Order locks the per-container restart sequence:
// pre_stop hooks → atomic ContainerRestart (not stop+start) → post_start
// hooks, with the Restarting event first and — a long-standing quirk — a
// final event that says Started, not Restarted.
func TestRestartContainer_Order(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)

	service := types.ServiceConfig{
		Name:      "web",
		PreStop:   []types.ServiceHook{{Command: types.ShellCommand{"quiesce"}}},
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"warmup"}}},
	}
	ctr := serviceContainer("web")

	// pre_stop exec
	preStop := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec-stop"}, nil)
	conn1s, conn1c := net.Pipe()
	_ = conn1s.Close()
	preStopAttach := apiClient.EXPECT().ExecAttach(gomock.Any(), "exec-stop", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(conn1c, "")}, nil).
		After(preStop)
	preStopInspect := apiClient.EXPECT().ExecInspect(gomock.Any(), "exec-stop", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 0}, nil).After(preStopAttach)

	restart := apiClient.EXPECT().ContainerRestart(gomock.Any(), ctr.ID, gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
			assert.Assert(t, rec.contains("Container prj-web-1: Restarting"))
			return client.ContainerRestartResult{}, nil
		}).After(preStopInspect)

	// post_start exec, after the restart
	postStart := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec-start"}, nil).After(restart)
	conn2s, conn2c := net.Pipe()
	_ = conn2s.Close()
	postStartAttach := apiClient.EXPECT().ExecAttach(gomock.Any(), "exec-start", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(conn2c, "")}, nil).
		After(postStart)
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec-start", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 0}, nil).After(postStartAttach)

	err := svc.restartContainer(t.Context(), service, ctr, api.RestartOptions{})
	assert.NilError(t, err)

	assert.DeepEqual(t, rec.summary(), []string{
		"Container prj-web-1: Restarting",
		"Container prj-web-1: Started",
	})
}

// A relay stands in for the service on the network but is a shell-less
// scratch binary: pre_stop/post_start have no process inside it to act on.
// No ExecCreate expectation is set: per newStartTestService, gomock fails
// the test if either hook still runs.
func TestRestartContainerSkipsHooksForRelay(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	service := types.ServiceConfig{
		Name:      "db",
		PreStop:   []types.ServiceHook{{Command: types.ShellCommand{"quiesce"}}},
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"warmup"}}},
	}
	relay := serviceContainer("db")
	relay.Labels[api.RelayLabel] = "abc123"

	apiClient.EXPECT().ContainerRestart(gomock.Any(), relay.ID, gomock.Any()).
		Return(client.ContainerRestartResult{}, nil)

	err := svc.restartContainer(t.Context(), service, relay, api.RestartOptions{})
	assert.NilError(t, err)
}

// TestRestart_ConcurrencyIsBoundedAcrossServices guards against a regression
// (PR #14177 review) where each service's per-container fan-out got its own
// maxConcurrency budget: with several independent services ready at once,
// the dependency-order traversal could run all of them in parallel, each
// spawning up to maxConcurrency ContainerRestart calls — up to N times the
// documented --parallel bound. The limiter must be shared across services.
func TestRestart_ConcurrencyIsBoundedAcrossServices(t *testing.T) {
	svc, apiClient := newTestService(t, WithMaxConcurrency(1))
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

	const numServices = 4
	project := &types.Project{Name: "prj", Services: types.Services{}}
	var containers Containers
	for i := range numServices {
		name := fmt.Sprintf("svc%d", i)
		project.Services[name] = types.ServiceConfig{Name: name}
		containers = append(containers, serviceContainer(name))
	}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: containers}, nil)

	tracker := &peakConcurrencyTracker{}
	apiClient.EXPECT().ContainerRestart(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
			tracker.leave()
			return client.ContainerRestartResult{}, nil
		}).
		Times(numServices)

	err := svc.restart(t.Context(), "prj", api.RestartOptions{Project: project})
	assert.NilError(t, err)
	assert.Equal(t, tracker.Peak(), 1, "restart must never run more than maxConcurrency ContainerRestart calls at once")
}

// TestRestart_WaitsForADependencyBeforeRestartingItsDependent pins the order
// restart gives a depends_on relation declaring restart:true: the dependency
// is restarted first, and its dependent waits for the condition to hold again
// -- here healthy -- before being restarted itself.
func TestRestart_WaitsForADependencyBeforeRestartingItsDependent(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	db := types.ServiceConfig{Name: "db"}
	web := types.ServiceConfig{Name: "web"}
	web.DependsOn = types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Restart: true, Required: true}}
	project := &types.Project{Name: "prj", Services: types.Services{"db": db, "web": web}}
	dbCtr := stoppedServiceContainer("db", 1, container.StateRunning)
	webCtr := stoppedServiceContainer("web", 1, container.StateRunning)

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{dbCtr, webCtr}}, nil)
	dbRestart := apiClient.EXPECT().ContainerRestart(gomock.Any(), dbCtr.ID, gomock.Any()).
		Return(client.ContainerRestartResult{}, nil)
	dbHealthy := apiClient.EXPECT().ContainerInspect(gomock.Any(), dbCtr.ID, gomock.Any()).
		Return(inspectResultOf(dbCtr.ID, "prj-db-1", container.StateRunning, 0, &container.Health{Status: container.Healthy}), nil).
		After(dbRestart)
	apiClient.EXPECT().ContainerRestart(gomock.Any(), webCtr.ID, gomock.Any()).
		Return(client.ContainerRestartResult{}, nil).After(dbHealthy)

	err := svc.restart(t.Context(), "prj", api.RestartOptions{Project: project})

	assert.NilError(t, err)
}

// TestRestartContainer_PassesTheTimeoutInWholeSeconds pins how the timeout
// given to restart reaches the daemon: in whole seconds, the unit the engine's
// API knows, and not at all when none was given, so that the container's own
// stop timeout applies.
func TestRestartContainer_PassesTheTimeoutInWholeSeconds(t *testing.T) {
	seconds := func(d time.Duration) *time.Duration { return &d }
	tests := []struct {
		name    string
		timeout *time.Duration
		want    *int
	}{
		{name: "none given", timeout: nil, want: nil},
		{name: "whole seconds", timeout: seconds(5 * time.Second), want: intPtr(5)},
		{name: "zero is a timeout too", timeout: seconds(0), want: intPtr(0)},
		{name: "a fraction of a second is dropped", timeout: seconds(2500 * time.Millisecond), want: intPtr(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, apiClient, _ := newStartTestService(t)
			ctr := serviceContainer("web")
			var got client.ContainerRestartOptions
			apiClient.EXPECT().ContainerRestart(gomock.Any(), ctr.ID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, opts client.ContainerRestartOptions) (client.ContainerRestartResult, error) {
					got = opts
					return client.ContainerRestartResult{}, nil
				})

			err := svc.restartContainer(t.Context(), types.ServiceConfig{Name: "web"}, ctr, api.RestartOptions{Timeout: tt.timeout})

			assert.NilError(t, err)
			assert.DeepEqual(t, got.Timeout, tt.want)
		})
	}
}
