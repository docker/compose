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
	"net"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// These tests characterize the restart path before the lifecycle engines
// converge (#14081): which depends_on relations survive project preparation,
// and the per-container sequence (pre_stop → ContainerRestart → post_start)
// with its events. restart stays on the imperative engine but shares the
// hook/wait primitives with the plan.

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
	ctr := serviceContainer("web", 1, container.StateRunning)

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
