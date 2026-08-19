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
	"strconv"
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// These tests characterize the imperative start path (startService,
// startServiceContainer) before it converges into the plan engine (#14081):
// which containers are started, in which order relative to file injection and
// hooks, when pre_start runs, and which progress events are emitted.

// recordingEventProcessor captures progress events for sequence assertions.
type recordingEventProcessor struct {
	mu     sync.Mutex
	events []api.Resource
}

func (r *recordingEventProcessor) Start(context.Context, string) {}

func (r *recordingEventProcessor) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

func (r *recordingEventProcessor) Done(string, bool) {}

// summary renders recorded events as "ID: Text" strings.
func (r *recordingEventProcessor) summary() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.ID+": "+e.Text)
	}
	return out
}

func (r *recordingEventProcessor) contains(entry string) bool {
	for _, e := range r.summary() {
		if e == entry {
			return true
		}
	}
	return false
}

// newStartTestService builds a composeService on gomock with a recording
// event processor. Any API call without a matching expectation fails the test,
// so "no expectation" doubles as a "no daemon interaction" assertion.
func newStartTestService(t *testing.T) (*composeService, *mocks.MockAPIClient, *recordingEventProcessor) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

	rec := &recordingEventProcessor{}
	svc, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	return svc.(*composeService), apiClient, rec
}

// runningContainer/stoppedContainer build container summaries the way the
// daemon reports project containers: canonical name, service and
// container-number labels.
func serviceContainer(service string, num int, state container.ContainerState) container.Summary {
	name := "prj-" + service + "-" + strconv.Itoa(num)
	return container.Summary{
		ID:    name + "-id",
		Names: []string{"/" + name},
		State: state,
		Labels: map[string]string{
			api.ServiceLabel:         service,
			api.ContainerNumberLabel: strconv.Itoa(num),
		},
	}
}

func TestStartService_AlreadyRunningIsSilent(t *testing.T) {
	svc, _, rec := newStartTestService(t)

	project := &types.Project{Name: "prj"}
	service := types.ServiceConfig{Name: "web"}
	containers := Containers{serviceContainer("web", 1, container.StateRunning)}

	// No expectation registered: any ContainerStart (or other call) fails.
	err := svc.startService(t.Context(), project, service, containers, nil, 0)
	assert.NilError(t, err)
	assert.Equal(t, len(rec.summary()), 0)
}

func TestStartService_ZeroReplicasIsNoop(t *testing.T) {
	svc, _, _ := newStartTestService(t)

	zero := 0
	project := &types.Project{Name: "prj"}
	service := types.ServiceConfig{Name: "web", Deploy: &types.DeployConfig{Replicas: &zero}}

	// Even with no containers at all, a zero-replicas service is not an error.
	err := svc.startService(t.Context(), project, service, nil, nil, 0)
	assert.NilError(t, err)
}

func TestStartService_NoContainers(t *testing.T) {
	svc, _, _ := newStartTestService(t)
	project := &types.Project{Name: "prj"}

	t.Run("scaled service is an error", func(t *testing.T) {
		service := types.ServiceConfig{Name: "web"}
		err := svc.startService(t.Context(), project, service, nil, nil, 0)
		assert.Error(t, err, `service "web" has no container to start`)
	})

	t.Run("scale zero is a no-op", func(t *testing.T) {
		service := types.ServiceConfig{Name: "web", Scale: intPtr(0)}
		err := svc.startService(t.Context(), project, service, nil, nil, 0)
		assert.NilError(t, err)
	})
}

// TestStartService_StartsOnlyStoppedReplicas locks three behaviors at once:
// only non-running replicas of the target service are started, containers of
// other services in the list are untouched, and pre_start does NOT run when at
// least one replica is already running.
func TestStartService_StartsOnlyStoppedReplicas(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)

	project := &types.Project{Name: "prj"}
	service := types.ServiceConfig{
		Name:     "web",
		PreStart: []types.ServiceHook{{Command: types.ShellCommand{"init"}}},
	}
	running := serviceContainer("web", 1, container.StateRunning)
	stopped := serviceContainer("web", 2, container.StateExited)
	other := serviceContainer("db", 1, container.StateExited)
	containers := Containers{running, stopped, other}

	// Only the stopped web replica is started; no ContainerCreate expectation
	// means any pre_start hook execution fails the test.
	apiClient.EXPECT().ContainerStart(gomock.Any(), stopped.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	err := svc.startService(t.Context(), project, service, containers, nil, 0)
	assert.NilError(t, err)

	assert.DeepEqual(t, rec.summary(), []string{
		"Container prj-web-2: Starting",
		"Container prj-web-2: Started",
	})
}

// TestStartService_PreStartOnLowestReplica locks the pre_start gating: with no
// replica running, the hooks run exactly once, against the replica with the
// lowest container-number — regardless of the order the daemon listed them in
// — and before any service container is started.
func TestStartService_PreStartOnLowestReplica(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	project := &types.Project{Name: "prj"}
	service := types.ServiceConfig{
		Name:     "web",
		Image:    "alpine",
		PreStart: []types.ServiceHook{{Command: types.ShellCommand{"init"}}},
	}
	// Listed out of order on purpose: replica 2 first.
	replica2 := serviceContainer("web", 2, container.StateExited)
	replica1 := serviceContainer("web", 1, container.StateExited)
	containers := Containers{replica2, replica1}

	// runPreStart sweeps orphan hook containers from any previous failed run
	// before creating the new one.
	orphanScan := apiClient.EXPECT().
		ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)

	// The hook container shares the volumes of the lowest-numbered replica.
	hookCreate := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		After(orphanScan).
		DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			assert.DeepEqual(t, opts.HostConfig.VolumesFrom, []string{replica1.ID})
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})
	hookWait := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(hookCreate)
	// streamPreStartLogs always opens ContainerLogs (even with nil listener) so
	// the tail is available for failure error messages.
	hookLogs := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(hookWait)
	hookStart := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(hookLogs)
	// On success the hook container is removed explicitly (AutoRemove is false).
	hookRemove := apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil).After(hookStart)

	// Replicas are then started sequentially, in list order, after the hook.
	start2 := apiClient.EXPECT().ContainerStart(gomock.Any(), replica2.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(hookRemove)
	apiClient.EXPECT().ContainerStart(gomock.Any(), replica1.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(start2)

	err := svc.startService(t.Context(), project, service, containers, nil, 0)
	assert.NilError(t, err)
}

// TestStartServiceContainer_Order locks the per-container start sequence:
// secret/config files are copied in before ContainerStart, post_start hooks
// run after it, and the Started event is only emitted once the hooks are done.
func TestStartServiceContainer_Order(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)

	content := "s3cret"
	project := &types.Project{
		Name: "prj",
		Secrets: types.Secrets{
			"token": types.SecretConfig{Name: "token", Content: content},
		},
	}
	service := types.ServiceConfig{
		Name:      "web",
		Secrets:   []types.ServiceSecretConfig{{Source: "token"}},
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"notify"}}},
	}
	ctr := serviceContainer("web", 1, container.StateExited)

	inject := apiClient.EXPECT().CopyToContainer(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.CopyToContainerResult{}, nil)
	start := apiClient.EXPECT().ContainerStart(gomock.Any(), ctr.ID, gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			assert.Assert(t, rec.contains("Container prj-web-1: Starting"))
			assert.Assert(t, !rec.contains("Container prj-web-1: Started"))
			return client.ContainerStartResult{}, nil
		}).After(inject)

	// post_start hook runs as an exec after the container started.
	execCreate := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec-1"}, nil).After(start)
	serverConn, clientConn := net.Pipe()
	_ = serverConn.Close()
	execAttach := apiClient.EXPECT().ExecAttach(gomock.Any(), "exec-1", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil).
		After(execCreate)
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec-1", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ExecInspectOptions) (client.ExecInspectResult, error) {
			// Started must not be emitted before post_start completed.
			assert.Assert(t, !rec.contains("Container prj-web-1: Started"))
			return client.ExecInspectResult{ExitCode: 0}, nil
		}).After(execAttach)

	err := svc.startServiceContainer(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)

	assert.DeepEqual(t, rec.summary(), []string{
		"Container prj-web-1: Starting",
		"Container prj-web-1: Started",
	})
}

// TestStartServiceContainer_FailedPostStartBlocksStarted locks that a failing
// post_start hook fails the start and leaves the Started event unemitted.
func TestStartServiceContainer_FailedPostStart(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)

	project := &types.Project{Name: "prj"}
	service := types.ServiceConfig{
		Name:      "web",
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"fail"}}},
	}
	ctr := serviceContainer("web", 1, container.StateExited)

	apiClient.EXPECT().ContainerStart(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec-1"}, nil)
	serverConn, clientConn := net.Pipe()
	_ = serverConn.Close()
	apiClient.EXPECT().ExecAttach(gomock.Any(), "exec-1", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil)
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec-1", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 3}, nil)

	err := svc.startServiceContainer(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "hook exited with status 3")
	assert.Assert(t, !rec.contains("Container prj-web-1: Started"))
}

// TestGetDependencyCondition locks the --wait condition selection: a service
// that others depend on with service_completed_successfully is waited on with
// that condition (or --wait would hang on one-shot services), anything else
// with running_or_healthy.
func TestGetDependencyCondition(t *testing.T) {
	oneShot := types.ServiceConfig{Name: "migrate"}
	web := types.ServiceConfig{
		Name: "web",
		DependsOn: types.DependsOnConfig{
			"migrate": {Condition: types.ServiceConditionCompletedSuccessfully},
		},
	}
	project := &types.Project{
		Name:     "prj",
		Services: types.Services{"web": web, "migrate": oneShot},
	}

	assert.Equal(t, getDependencyCondition(oneShot, project), types.ServiceConditionCompletedSuccessfully)
	assert.Equal(t, getDependencyCondition(web, project), ServiceConditionRunningOrHealthy)
}
