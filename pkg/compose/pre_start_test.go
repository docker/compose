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
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func newPreStartTestService(t *testing.T) (*composeService, *mocks.MockAPIClient) {
	t.Helper()
	// Register the goroutine-leak check first so it runs last (t.Cleanup is
	// LIFO), after gomock's own cleanup has drained any internal goroutines.
	// Any goroutine spawned by the code under test that outlives the test will
	// fail this assertion.
	ignoreExisting := goleak.IgnoreCurrent()
	t.Cleanup(func() {
		goleak.VerifyNone(t, ignoreExisting)
	})
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	return tested.(*composeService), apiClient
}

func waitResultExit(code int64) client.ContainerWaitResult {
	resultC := make(chan container.WaitResponse, 1)
	errC := make(chan error, 1)
	resultC <- container.WaitResponse{StatusCode: code}
	return client.ContainerWaitResult{Result: resultC, Error: errC}
}

func emptyLogs() client.ContainerLogsResult {
	return io.NopCloser(bytes.NewReader(nil))
}

func TestPreStart_SuccessTwoHooksInOrder(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"echo", "first"}},
			{Image: "alpine", Command: types.ShellCommand{"echo", "second"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	// Hook 1: create → wait (subscribe) → logs (subscribe) → start.
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)

	// Hook 2 is only created after hook 1 has been started (and waited on).
	create2 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-2"}, nil).After(start1)
	wait2 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-2", gomock.Any()).
		Return(waitResultExit(0)).After(create2)
	logs2 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-2", gomock.Any()).
		Return(emptyLogs(), nil).After(wait2)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-2", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs2)

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
}

func TestPreStart_FirstHookFailsStopsExecution(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"false"}},
			{Image: "alpine", Command: types.ShellCommand{"echo", "never"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(42)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "42")
}

func TestPreStart_PerReplicaRejected(t *testing.T) {
	tested, _ := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}, PerReplica: true},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "per_replica is not yet supported")
}

func TestPreStart_ImageFallsBackToBuiltImage(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	// Service with no explicit image (build-only); hook image also empty.
	service := types.ServiceConfig{
		Name: "web",
		PreStart: []types.ServiceHook{
			{Command: types.ShellCommand{"echo", "hi"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	var gotImage string
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotImage = opts.Config.Image
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
	assert.Equal(t, gotImage, api.GetImageNameOrDefault(service, project.Name))
}

func TestPreStart_ExplicitHookImageUsed(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "service-image:latest",
		PreStart: []types.ServiceHook{
			{Image: "custom-hook-image:1.2.3", Command: types.ShellCommand{"echo"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	var gotImage string
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotImage = opts.Config.Image
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
	assert.Equal(t, gotImage, "custom-hook-image:1.2.3")
}

func TestPreStart_VolumesFromServiceContainer(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	var gotVolumesFrom []string
	var gotAutoRemove bool
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotVolumesFrom = opts.HostConfig.VolumesFrom
			gotAutoRemove = opts.HostConfig.AutoRemove
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
	assert.DeepEqual(t, gotVolumesFrom, []string{"service-ctr-id"})
	assert.Assert(t, gotAutoRemove)
}

func TestPreStart_ExtraHostsPassedToContainer(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		ExtraHosts: types.HostsList{
			"somehost": {"162.242.195.82"},
		},
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	var gotExtraHosts []string
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotExtraHosts = opts.HostConfig.ExtraHosts
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
	assert.DeepEqual(t, gotExtraHosts, []string{"somehost:162.242.195.82"})
}

func TestPreStart_ContainerCreateFailurePropagates(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "missing:latest", Command: types.ShellCommand{"true"}},
			{Image: "alpine", Command: types.ShellCommand{"never"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{}, fmt.Errorf("no such image: missing:latest"))

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.ErrorContains(t, err, "no such image")
}

func TestPreStart_ContainerStartFailurePropagates(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, fmt.Errorf("daemon: container start failed")).After(logs1)
	// AutoRemove never fires when start fails, so the hook must drop the ghost
	// container explicitly.
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil).After(start1)

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.ErrorContains(t, err, "container start failed")
}

// TestPreStart_WaitResultPreferredOverNilError pins the fix for the scheduler
// race in waitPreStart: when ContainerWait closes a successful stream cleanly
// it delivers Result (exit code) AND a nil send on Error at the same time.
// A naive 3-case select would pick Error half the time and turn the run into
// a spurious "wait ended without an exit status" failure. The function must
// always settle on the Result-based outcome.
func TestPreStart_WaitResultPreferredOverNilError(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	// Both channels are buffered and pre-populated so the outer select in
	// waitPreStart sees them ready at the same instant.
	resultC := make(chan container.WaitResponse, 1)
	errC := make(chan error, 1)
	resultC <- container.WaitResponse{StatusCode: 0}
	errC <- nil

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerWaitResult{Result: resultC, Error: errC})
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
}

// TestWaitPreStart_RaceNilErrorAndResult stress-tests the scheduler outcome
// when ContainerWait closes a successful stream cleanly: Result has the exit
// code and Error sends nil at the same instant. Either branch of the outer
// select must end on the Result-based success, with no spurious failure.
func TestWaitPreStart_RaceNilErrorAndResult(t *testing.T) {
	for i := 0; i < 100; i++ {
		resultC := make(chan container.WaitResponse, 1)
		errC := make(chan error, 1)
		resultC <- container.WaitResponse{StatusCode: 0}
		errC <- nil
		waitRes := client.ContainerWaitResult{Result: resultC, Error: errC}
		assert.NilError(t, waitPreStart(t.Context(), "web", 0, waitRes))
	}
}

// TestWaitPreStart_RaceRealErrorAndResult stress-tests the opposite scenario:
// a real transport error on Error races with a stale Result. The Error must
// always win — the function must never silently drop the failure and return
// success based on Result.
func TestWaitPreStart_RaceRealErrorAndResult(t *testing.T) {
	for i := 0; i < 100; i++ {
		resultC := make(chan container.WaitResponse, 1)
		errC := make(chan error, 1)
		resultC <- container.WaitResponse{StatusCode: 0}
		errC <- fmt.Errorf("daemon: connection lost")
		waitRes := client.ContainerWaitResult{Result: resultC, Error: errC}
		err := waitPreStart(t.Context(), "web", 0, waitRes)
		assert.ErrorContains(t, err, "connection lost")
	}
}
