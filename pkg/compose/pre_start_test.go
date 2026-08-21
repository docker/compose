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
	"context"
	"fmt"
	"io"
	"strings"
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
	return newPreStartTestServiceWithVersion(t, "1.44")
}

func newPreStartTestServiceWithVersion(t *testing.T, apiVersion string) (*composeService, *mocks.MockAPIClient) {
	t.Helper()
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
		Return(client.PingResult{APIVersion: apiVersion}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return(apiVersion).AnyTimes()
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

// expectEmptyOrphanScan sets up the ContainerList expectation for the orphan
// pre_start cleanup that happens once at the start of every runPreStart call
// (after the per_replica validation loop). It returns the empty list.
func expectEmptyOrphanScan(apiClient *mocks.MockAPIClient) *gomock.Call {
	return apiClient.EXPECT().
		ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)
}

// expectSuccessRemove sets up the ContainerRemove call that runPreStartHook
// makes after a successful hook run (mirrors old AutoRemove behaviour).
func expectSuccessRemove(apiClient *mocks.MockAPIClient, hookID string) *gomock.Call {
	return apiClient.EXPECT().
		ContainerRemove(gomock.Any(), hookID, client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)
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

	// Orphan cleanup runs once before the first hook.
	scan := expectEmptyOrphanScan(apiClient)

	// Hook 1: create → wait (subscribe) → logs (subscribe) → start → remove.
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	remove1 := expectSuccessRemove(apiClient, "hook-1").After(start1)

	// Hook 2 is only created after hook 1 has been removed.
	create2 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-2"}, nil).After(remove1)
	wait2 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-2", gomock.Any()).
		Return(waitResultExit(0)).After(create2)
	logs2 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-2", gomock.Any()).
		Return(emptyLogs(), nil).After(wait2)
	start2 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-2", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs2)
	expectSuccessRemove(apiClient, "hook-2").After(start2)

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

	scan := expectEmptyOrphanScan(apiClient)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(42)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	// Hook container is retained on failure — no ContainerRemove expected.

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
	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotImage = opts.Config.Image
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		}).After(scan)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	expectSuccessRemove(apiClient, "hook-1")

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
	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotImage = opts.Config.Image
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		}).After(scan)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	expectSuccessRemove(apiClient, "hook-1")

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
	var gotLabels map[string]string
	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotVolumesFrom = opts.HostConfig.VolumesFrom
			gotAutoRemove = opts.HostConfig.AutoRemove
			gotLabels = opts.Config.Labels
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		}).After(scan)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, func(api.ContainerEvent) {})
	assert.NilError(t, err)
	assert.DeepEqual(t, gotVolumesFrom, []string{"service-ctr-id"})
	// AutoRemove must be false: the hook container is retained on failure for
	// post-mortem and explicitly removed on success by runPreStartHook.
	assert.Assert(t, !gotAutoRemove, "AutoRemove must be false")
	// HookLabel must be set so orphan cleanup can identify the container.
	assert.Equal(t, gotLabels[api.HookLabel], preStartHookType)
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

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{}, fmt.Errorf("no such image: missing:latest")).After(scan)

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

	scan := expectEmptyOrphanScan(apiClient)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, fmt.Errorf("daemon: container start failed")).After(logs1)
	// AutoRemove never fires when start fails, so the hook must drop the ghost
	// container explicitly. This is distinct from the success-path removal
	// (RemoveVolumes:true) — the never-started container has no logs to preserve.
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
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

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerWaitResult{Result: resultC, Error: errC})
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

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

// stdcopyFrame encodes a single stdcopy-multiplexed frame.
// stream: 1=stdout, 2=stderr. Returns the encoded bytes.
// This helper is kept local (not using writeStdcopyFrame from hook_test.go)
// because hook_test.go carries a //go:build !windows constraint.
func stdcopyFrame(stream byte, data string) []byte {
	payload := []byte(data)
	header := [8]byte{stream, 0, 0, 0}
	header[4] = byte(len(payload) >> 24)
	header[5] = byte(len(payload) >> 16)
	header[6] = byte(len(payload) >> 8)
	header[7] = byte(len(payload))
	return append(header[:], payload...)
}

// TestPreStart_DetachedModeAttachesLogs verifies that ContainerLogs is opened
// even when listener is nil (detached / compose up -d). Previously
// streamPreStartLogs short-circuited and returned immediately, leaving the tail
// buffer empty and any failure error without output context.
func TestPreStart_DetachedModeAttachesLogs(t *testing.T) {
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

	scan := expectEmptyOrphanScan(apiClient)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	// ContainerLogs MUST be called even with a nil listener.
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_FailureIncludesTail verifies that when a pre_start hook exits
// with a non-zero code the error message includes the captured stderr output,
// giving operators the context they need to diagnose the failure.
func TestPreStart_FailureIncludesTail(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "db",
		Image: "postgres",
		PreStart: []types.ServiceHook{
			{Image: "postgres", Command: types.ShellCommand{"migrate"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	// Build a stdcopy-multiplexed log stream with a stderr error line.
	logContent := append(
		stdcopyFrame(1, "starting migration\n"),             // stdout noise
		stdcopyFrame(2, "table 'sites' doesn't exist\n")..., // stderr error
	)

	scan := expectEmptyOrphanScan(apiClient)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	// Hook container is retained on failure — no ContainerRemove expected.

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.Assert(t, err != nil)
	assert.ErrorContains(t, err, "pre_start[0]")
	// Stderr content must be in the error (stderr bias).
	assert.ErrorContains(t, err, "doesn't exist")
}

// ---------------------------------------------------------------------------
// Feature tests: pre_start container retention on failure
// ---------------------------------------------------------------------------

// TestPreStart_SuccessRemovesContainer verifies that a successful pre_start hook
// triggers an explicit ContainerRemove (with RemoveVolumes: true to mirror the
// old AutoRemove behaviour) and that AutoRemove is false at create time.
func TestPreStart_SuccessRemovesContainer(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	var gotAutoRemove bool
	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotAutoRemove = opts.HostConfig.AutoRemove
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		}).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// Success path: explicit remove with RemoveVolumes: true.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
	assert.Assert(t, !gotAutoRemove, "AutoRemove must be false; retention is managed explicitly")
}

// TestPreStart_FailureRetainsContainer verifies that a pre_start hook that exits
// with a non-zero code does NOT trigger ContainerRemove — the container is kept
// for post-mortem inspection.
func TestPreStart_FailureRetainsContainer(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"migrate"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	logContent := stdcopyFrame(2, "migration failed: table missing\n")

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// No ContainerRemove expectation: gomock fails on unexpected calls.

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "pre_start[0]")
	assert.ErrorContains(t, err, "table missing")
	// Short container ID must appear in the error so the operator can run
	// `docker logs hook-1` immediately.
	assert.ErrorContains(t, err, "hook-1")
}

// TestPreStart_CancellationRemovesContainer verifies that when the context is
// cancelled while a hook is running the container is removed (not retained) and
// the error is exactly ctx.Err() with no tail decoration.
func TestPreStart_CancellationRemovesContainer(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	ctx, cancel := context.WithCancel(t.Context())

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"long-running-op"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-cancel-123"}, nil).After(scan)
	// ContainerWait channel: cancel the context before delivering any result so
	// waitPreStart returns ctx.Err().
	resultC := make(chan container.WaitResponse)
	errC := make(chan error)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-cancel-123", gomock.Any()).
		Return(client.ContainerWaitResult{Result: resultC, Error: errC})
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-cancel-123", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-cancel-123", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// On cancellation the container must be removed, not retained.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-cancel-123", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- tested.runPreStart(ctx, project, service, ctr, nil)
	}()
	// Cancel after the hook container has started.
	cancel()

	err := <-errCh
	assert.ErrorIs(t, err, context.Canceled, "must return ctx.Err() on cancellation")
	// The error must NOT include "retained" — it should be the raw context error.
	assert.Assert(t, !strings.Contains(err.Error(), "retained"), "cancelled hook must not be retained; got: %s", err)
}

// TestPreStart_RemovesOrphanBeforeRun verifies that a hook container left behind
// by a previous failed run is force-removed before the new container is created.
func TestPreStart_RemovesOrphanBeforeRun(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"migrate"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	// ContainerList returns the stale orphan from a previous run.
	orphan := container.Summary{ID: "orphan-hook-123", State: "exited"}
	scan := apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{orphan}}, nil)
	// Orphan must be removed before the new hook container is created.
	removeOrphan := apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "orphan-hook-123", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil).After(scan)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(removeOrphan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_SuccessRemoveFailureIsNonFatal verifies that a ContainerRemove
// failure on the success path is logged but does not fail runPreStart.
func TestPreStart_SuccessRemoveFailureIsNonFatal(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// Simulate a removal failure.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, fmt.Errorf("already removed"))

	// The hook succeeded; the service must start even if removal failed.
	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// ---------------------------------------------------------------------------
// Coverage-gap tests: previously uncovered branches
// ---------------------------------------------------------------------------

// TestLowestNumberedContainer verifies that lowestNumberedContainer always
// picks the replica with the smallest ContainerNumberLabel value.
func TestLowestNumberedContainer(t *testing.T) {
	makeCtr := func(id, num string) container.Summary {
		return container.Summary{ID: id, Labels: map[string]string{
			api.ContainerNumberLabel: num,
		}}
	}
	tests := []struct {
		name       string
		containers Containers
		wantID     string
	}{
		{"single", Containers{makeCtr("a", "1")}, "a"},
		{"ascending", Containers{makeCtr("a", "1"), makeCtr("b", "2"), makeCtr("c", "3")}, "a"},
		{"descending", Containers{makeCtr("a", "3"), makeCtr("b", "2"), makeCtr("c", "1")}, "c"},
		{"unordered", Containers{makeCtr("a", "5"), makeCtr("b", "1"), makeCtr("c", "3")}, "b"},
		{"missing_label", Containers{makeCtr("x", ""), makeCtr("y", "1")}, "x"}, // strconv.Atoi("") = 0 = lowest
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lowestNumberedContainer(tc.containers)
			assert.Equal(t, got.ID, tc.wantID)
		})
	}
}

// TestPreStart_WaitCancelled covers the ctx.Done() branch in waitPreStart:
// when the context is already canceled the function must return ctx.Err()
// without blocking on the result or error channels.
func TestPreStart_WaitCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already canceled

	resultC := make(chan container.WaitResponse) // never sends
	errC := make(chan error)                     // never sends
	waitRes := client.ContainerWaitResult{Result: resultC, Error: errC}

	err := waitPreStart(ctx, "web", 0, waitRes)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestPreStart_ResultErrWaitError covers the preStartResultErr branch where
// the WaitResponse carries a daemon-reported error message.
func TestPreStart_ResultErrWaitError(t *testing.T) {
	res := container.WaitResponse{
		Error: &container.WaitExitError{Message: "oom killed"},
	}
	err := preStartResultErr("web", 0, res)
	assert.ErrorContains(t, err, "wait error")
	assert.ErrorContains(t, err, "oom killed")
}

// TestPreStart_StreamLogsError_NilListener covers the ContainerLogs-failure
// branch in streamPreStartLogs when the caller passes no listener.
// The hook must still start and succeed; the missing log stream is non-fatal.
func TestPreStart_StreamLogsError_NilListener(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	// ContainerLogs fails; nil listener → no warning event, done closed immediately.
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(nil, fmt.Errorf("logs: connection refused"))
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_StreamLogsError_WithListener covers the ContainerLogs-failure
// branch in streamPreStartLogs when a listener IS present: a warning event
// must be emitted on the listener.
func TestPreStart_StreamLogsError_WithListener(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(nil, fmt.Errorf("logs: daemon unavailable"))
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	var gotWarning string
	listener := func(ev api.ContainerEvent) {
		gotWarning = ev.Line
	}
	err := tested.runPreStart(t.Context(), project, service, ctr, listener)
	assert.NilError(t, err)
	assert.Assert(t, gotWarning != "", "listener must receive a warning when ContainerLogs fails")
	assert.Assert(t, bytes.Contains([]byte(gotWarning), []byte("warning")), "expected 'warning' in: %q", gotWarning)
}

// TestPreStart_OldAPIVersion covers the versions.LessThan(apiVersion, "1.44")
// branch in createPreStartContainer: on a pre-1.44 daemon the extra-networks
// path runs via connectPreStartExtraNetworks. With only one (primary) network
// no NetworkConnect call is issued and the hook succeeds normally.
func TestPreStart_OldAPIVersion(t *testing.T) {
	tested, apiClient := newPreStartTestServiceWithVersion(t, "1.43")

	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"default": {Name: "proj_default"},
		},
	}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// Single network = primary only; no NetworkConnect expected.
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_ConnectExtraNetworksSuccess covers connectPreStartExtraNetworks
// when a secondary network is present: NetworkConnect must be called for it.
func TestPreStart_ConnectExtraNetworksSuccess(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"default": {Name: "proj_default"},
			"extra":   {Name: "proj_extra"},
		},
	}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
			"extra":   nil,
		},
	}

	apiClient.EXPECT().
		NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, nil)

	err := tested.connectPreStartExtraNetworks(t.Context(), project, service, "ctr-id", "proj_default")
	assert.NilError(t, err)
}

// TestPreStart_ConnectExtraNetworksFails covers the NetworkConnect error branch
// in connectPreStartExtraNetworks.
func TestPreStart_ConnectExtraNetworksFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"default": {Name: "proj_default"},
			"extra":   {Name: "proj_extra"},
		},
	}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
			"extra":   nil,
		},
	}

	apiClient.EXPECT().
		NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, fmt.Errorf("network not found"))

	err := tested.connectPreStartExtraNetworks(t.Context(), project, service, "ctr-id", "proj_default")
	assert.ErrorContains(t, err, "network not found")
}

// TestPreStart_ContainerStartFailureAndRemoveFails covers the Warnf path in
// runPreStartHook when ContainerStart fails AND the subsequent ContainerRemove
// also fails (the orphan is unremovable but the caller still gets the start error).
func TestPreStart_ContainerStartFailureAndRemoveFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	create1 := apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(create1)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, fmt.Errorf("start failed")).After(logs1)
	// Both the start AND the cleanup removal fail.
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, fmt.Errorf("removal failed")).After(start1)

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	// The original start error must be returned, not the removal error.
	assert.ErrorContains(t, err, "start failed")
}

// TestPreStart_OrphanScanFails verifies that when ContainerList fails during
// orphan cleanup the Warnf path in runPreStart is hit, but execution continues
// and the hook runs successfully.
func TestPreStart_OrphanScanFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	// ContainerList fails → Warnf in runPreStart; hook still proceeds.
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, fmt.Errorf("daemon unavailable"))
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_OrphanRemovalFails verifies the Warnf path in
// removeOrphanPreStartContainers when an individual stale container cannot be
// removed. The hook must still proceed normally.
func TestPreStart_OrphanRemovalFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	orphan := container.Summary{ID: "orphan-123"}
	// ContainerList finds the orphan; its removal fails → Warnf then continue.
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{orphan}}, nil)
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "orphan-123", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, fmt.Errorf("permission denied"))
	// Hook still runs and succeeds.
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.NilError(t, err)
}

// TestPreStart_OldAPINetworkConnectFails covers the createPreStartContainer path
// for API < 1.44 with a secondary network: when NetworkConnect fails the function
// must clean up the created-but-never-started container and return the error.
func TestPreStart_OldAPINetworkConnectFails(t *testing.T) {
	tested, apiClient := newPreStartTestServiceWithVersion(t, "1.43")

	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"default": {Name: "proj_default"},
			"extra":   {Name: "proj_extra"},
		},
	}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
			"extra":   nil,
		},
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	// NetworkConnect for the secondary network fails.
	apiClient.EXPECT().NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, fmt.Errorf("network not found"))
	// The never-started container is cleaned up (Force:true).
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "network not found")
}

// TestPreStart_OldAPINetworkConnectAndRemoveFails covers the Warnf path inside
// createPreStartContainer when both NetworkConnect and the cleanup ContainerRemove fail.
func TestPreStart_OldAPINetworkConnectAndRemoveFails(t *testing.T) {
	tested, apiClient := newPreStartTestServiceWithVersion(t, "1.43")

	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"default": {Name: "proj_default"},
			"extra":   {Name: "proj_extra"},
		},
	}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
			"extra":   nil,
		},
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, fmt.Errorf("network not found"))
	// Cleanup removal also fails → Warnf; original error is still returned.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, fmt.Errorf("removal also failed"))

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "network not found")
}

// TestPreStart_RuntimeAPIVersionError covers the early-return in
// createPreStartContainer when RuntimeAPIVersion fails (Ping returns an error).
func TestPreStart_RuntimeAPIVersionError(t *testing.T) {
	ignoreExisting := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignoreExisting) })

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	s := tested.(*composeService)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	// Orphan scan succeeds (ContainerList does not need Ping).
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)
	// createPreStartContainer calls RuntimeAPIVersion → Ping fails.
	apiClient.EXPECT().
		Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{}, fmt.Errorf("daemon unreachable"))

	err = s.runPreStart(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "daemon unreachable")
}

// TestPreStart_FailureStdoutOnlyTail covers the stdout-fallback branch in the
// getTail closure inside streamPreStartLogs: when stderr is empty but stdout has
// content, getTail must return the stdout content.
func TestPreStart_FailureStdoutOnlyTail(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "db",
		Image: "postgres",
		PreStart: []types.ServiceHook{
			{Image: "postgres", Command: types.ShellCommand{"migrate"}},
		},
	}
	ctr := container.Summary{ID: "svc-ctr"}

	// stdout-only content: stderr frame absent so getTail falls back to stdout.
	logContent := stdcopyFrame(1, "migration failed: schema mismatch\n")

	scan := expectEmptyOrphanScan(apiClient)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil).After(scan)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1))
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, ctr, nil)
	assert.ErrorContains(t, err, "pre_start[0]")
	// Stdout fallback: no stderr → stdout content appears in the error.
	assert.ErrorContains(t, err, "schema mismatch")
}
