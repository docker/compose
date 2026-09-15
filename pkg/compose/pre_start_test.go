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
	"errors"
	"io"
	"strconv"
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

// runnerSummary builds the container.Summary of a hook runner as prepared by
// the reconciliation plan: created state, hook labels carrying the index.
func runnerSummary(id string, index int) container.Summary {
	return container.Summary{
		ID:    id,
		State: container.StateCreated,
		Labels: map[string]string{
			api.HookLabel:      preStartHookType,
			api.HookIndexLabel: strconv.Itoa(index),
		},
	}
}

// expectRunnerScan sets up the ContainerList expectation for the runner lookup
// that happens once at the start of every runPreStart call (after the
// per_replica validation loop). It returns the given runners.
func expectRunnerScan(apiClient *mocks.MockAPIClient, runners ...container.Summary) *gomock.Call {
	return apiClient.EXPECT().
		ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: runners}, nil)
}

// expectSuccessRemove sets up the ContainerRemove call that runPreStart
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

	// Both runners were prepared by the plan; the scan runs once up front.
	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0), runnerSummary("hook-2", 1))

	// Hook 1: wait (subscribe) → logs (subscribe) → start → remove.
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	remove1 := expectSuccessRemove(apiClient, "hook-1").After(start1)

	// Hook 2 only runs after hook 1 has been removed.
	wait2 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-2", gomock.Any()).
		Return(waitResultExit(0)).After(remove1)
	logs2 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-2", gomock.Any()).
		Return(emptyLogs(), nil).After(wait2)
	start2 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-2", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs2)
	expectSuccessRemove(apiClient, "hook-2").After(start2)

	err := tested.runPreStart(t.Context(), project, service, func(api.ContainerEvent) {})
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0), runnerSummary("hook-2", 1))
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(42)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	// Hook container is retained on failure — no ContainerRemove expected, and
	// hook-2's runner is never touched.

	err := tested.runPreStart(t.Context(), project, service, func(api.ContainerEvent) {})
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

	err := tested.runPreStart(t.Context(), project, service, func(api.ContainerEvent) {})
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "per_replica is not yet supported")
}

// ---------------------------------------------------------------------------
// Pure-execution contract: runPreStart never creates a runner
// ---------------------------------------------------------------------------

// TestPreStart_MissingRunnerFails pins the no-fallback contract: a declared
// hook without a prepared runner is an error pointing the user at the
// reconciliation command, not an implicit creation. This is what a user sees
// on `stop` then `start`: the runners were consumed by the first start.
func TestPreStart_MissingRunnerFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}

	expectRunnerScan(apiClient)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "docker compose up web")
}

// TestPreStart_ConsumedRunnerNotReused verifies that a runner in any state but
// created (here: exited, e.g. retained after a failure) is not re-executed —
// the hook reports the runner as missing instead of re-running a stale one.
func TestPreStart_ConsumedRunnerNotReused(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}

	consumed := runnerSummary("hook-old", 0)
	consumed.State = container.StateExited
	expectRunnerScan(apiClient, consumed)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "docker compose up web")
}

// TestPreStart_OldGenerationRunnerIgnored verifies that a hook container
// without a HookIndexLabel (created by an older compose version) is never
// matched to a declared hook: the purge planned by the reconciler is the only
// consumer of those containers.
func TestPreStart_OldGenerationRunnerIgnored(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}

	legacy := container.Summary{
		ID:     "legacy-hook",
		State:  container.StateCreated,
		Labels: map[string]string{api.HookLabel: preStartHookType},
	}
	expectRunnerScan(apiClient, legacy)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, `service "web" pre_start[0]`)
	assert.ErrorContains(t, err, "docker compose up web")
}

// TestPreStart_SecondRunnerMissing verifies that a missing runner is only
// reported when its hook is reached: the first hook runs (and is consumed)
// before pre_start[1] fails.
func TestPreStart_SecondRunnerMissing(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	expectSuccessRemove(apiClient, "hook-1").After(start1)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, `service "web" pre_start[1]`)
	assert.ErrorContains(t, err, "docker compose up web")
}

// TestPreStart_RunnerScanFails verifies that a ContainerList failure during
// the runner lookup is fatal: without the runner set runPreStart cannot tell
// prepared hooks from missing ones.
func TestPreStart_RunnerScanFails(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "proj"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, errors.New("daemon unavailable"))

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, "daemon unavailable")
}

// ---------------------------------------------------------------------------
// Runner creation (createPreStartContainer, called by the plan executor)
// ---------------------------------------------------------------------------

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

	created, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "demo-web-pre_start-0")
	assert.NilError(t, err)
	assert.Equal(t, created.ID, "hook-1")
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

	_, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "demo-web-pre_start-0")
	assert.NilError(t, err)
	assert.Equal(t, gotImage, "custom-hook-image:1.2.3")
}

// TestPreStart_CreateNameAndLabels pins the runner's plan-visible identity:
// deterministic container name, VolumesFrom on the target replica, AutoRemove
// off (retention is managed explicitly) and the hook labels — type for the
// purge, index for the start-phase lookup.
func TestPreStart_CreateNameAndLabels(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "alpine", Command: types.ShellCommand{"true"}},
			{Image: "alpine", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	var gotOpts client.ContainerCreateOptions
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotOpts = opts
			return client.ContainerCreateResult{ID: "hook-1"}, nil
		})

	name := getHookContainerName(project.Name, service.Name, 1)
	_, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 1, name)
	assert.NilError(t, err)
	assert.Equal(t, gotOpts.Name, "demo-web-pre_start-1")
	assert.DeepEqual(t, gotOpts.HostConfig.VolumesFrom, []string{"service-ctr-id"})
	assert.Assert(t, !gotOpts.HostConfig.AutoRemove, "AutoRemove must be false")
	assert.Equal(t, gotOpts.Config.Labels[api.HookLabel], preStartHookType)
	assert.Equal(t, gotOpts.Config.Labels[api.HookIndexLabel], "1")
}

func TestPreStart_ContainerCreateFailurePropagates(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:  "web",
		Image: "alpine",
		PreStart: []types.ServiceHook{
			{Image: "missing:latest", Command: types.ShellCommand{"true"}},
		},
	}
	ctr := container.Summary{ID: "service-ctr-id"}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{}, errors.New("no such image: missing:latest"))

	_, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "demo-web-pre_start-0")
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, errors.New("daemon: container start failed")).After(logs1)
	// AutoRemove never fires when start fails, so the hook must drop the ghost
	// container explicitly. This is distinct from the success-path removal
	// (RemoveVolumes:true) — the never-started container has no logs to preserve.
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil).After(start1)

	err := tested.runPreStart(t.Context(), project, service, func(api.ContainerEvent) {})
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

	// Both channels are buffered and pre-populated so the outer select in
	// waitPreStart sees them ready at the same instant.
	resultC := make(chan container.WaitResponse, 1)
	errC := make(chan error, 1)
	resultC <- container.WaitResponse{StatusCode: 0}
	errC <- nil

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerWaitResult{Result: resultC, Error: errC}).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, func(api.ContainerEvent) {})
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
		errC <- errors.New("daemon: connection lost")
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	// ContainerLogs MUST be called even with a nil listener.
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, nil)
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

	// Build a stdcopy-multiplexed log stream with a stderr error line.
	logContent := append(
		stdcopyFrame(1, "starting migration\n"),             // stdout noise
		stdcopyFrame(2, "table 'sites' doesn't exist\n")..., // stderr error
	)

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	wait1 := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil).After(wait1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs1)
	// Hook container is retained on failure — no ContainerRemove expected.

	err := tested.runPreStart(t.Context(), project, service, nil)
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
// old AutoRemove behaviour).
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// Success path: explicit remove with RemoveVolumes: true.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.NilError(t, err)
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

	logContent := stdcopyFrame(2, "migration failed: table missing\n")

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1)).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// No ContainerRemove expectation: gomock fails on unexpected calls.

	err := tested.runPreStart(t.Context(), project, service, nil)
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-cancel-123", 0))
	// ContainerWait channel: cancel the context before delivering any result so
	// waitPreStart returns ctx.Err().
	resultC := make(chan container.WaitResponse)
	errC := make(chan error)
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-cancel-123", gomock.Any()).
		Return(client.ContainerWaitResult{Result: resultC, Error: errC}).After(scan)
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
		errCh <- tested.runPreStart(ctx, project, service, nil)
	}()
	// Cancel after the hook container has started.
	cancel()

	err := <-errCh
	assert.ErrorIs(t, err, context.Canceled, "must return ctx.Err() on cancellation")
	// The error must NOT include "retained" — it should be the raw context error.
	assert.Assert(t, !strings.Contains(err.Error(), "retained"), "cancelled hook must not be retained; got: %s", err)
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	// Simulate a removal failure.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, errors.New("already removed"))

	// The hook succeeded; the service must start even if removal failed.
	err := tested.runPreStart(t.Context(), project, service, nil)
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	// ContainerLogs fails; nil listener → no warning event, done closed immediately.
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(nil, errors.New("logs: connection refused"))
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	err := tested.runPreStart(t.Context(), project, service, nil)
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(nil, errors.New("logs: daemon unavailable"))
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	expectSuccessRemove(apiClient, "hook-1")

	var gotWarning string
	listener := func(ev api.ContainerEvent) {
		gotWarning = ev.Line
	}
	err := tested.runPreStart(t.Context(), project, service, listener)
	assert.NilError(t, err)
	assert.Assert(t, gotWarning != "", "listener must receive a warning when ContainerLogs fails")
	assert.Assert(t, bytes.Contains([]byte(gotWarning), []byte("warning")), "expected 'warning' in: %q", gotWarning)
}

// TestPreStart_OldAPIVersion covers the versions.LessThan(apiVersion, "1.44")
// branch in createPreStartContainer: on a pre-1.44 daemon the extra-networks
// path runs via connectPreStartExtraNetworks. With only one (primary) network
// no NetworkConnect call is issued and the create succeeds normally.
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

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	// Single network = primary only; no NetworkConnect expected.

	created, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "proj-web-pre_start-0")
	assert.NilError(t, err)
	assert.Equal(t, created.ID, "hook-1")
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
		Return(client.NetworkConnectResult{}, errors.New("network not found"))

	err := tested.connectPreStartExtraNetworks(t.Context(), project, service, "ctr-id", "proj_default")
	assert.ErrorContains(t, err, "network not found")
}

// TestPreStart_ContainerStartFailureAndRemoveFails covers the Warnf path in
// execPreStartHook when ContainerStart fails AND the subsequent ContainerRemove
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

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	logs1 := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil)
	start1 := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, errors.New("start failed")).After(logs1)
	// Both the start AND the cleanup removal fail.
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, errors.New("removal failed")).After(start1)

	err := tested.runPreStart(t.Context(), project, service, nil)
	// The original start error must be returned, not the removal error.
	assert.ErrorContains(t, err, "start failed")
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

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	// NetworkConnect for the secondary network fails.
	apiClient.EXPECT().NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, errors.New("network not found"))
	// The never-started container is cleaned up (Force:true).
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	_, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "proj-web-pre_start-0")
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

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "hook-1"}, nil)
	apiClient.EXPECT().NetworkConnect(gomock.Any(), "proj_extra", gomock.Any()).
		Return(client.NetworkConnectResult{}, errors.New("network not found"))
	// Cleanup removal also fails → Warnf; original error is still returned.
	apiClient.EXPECT().
		ContainerRemove(gomock.Any(), "hook-1", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, errors.New("removal also failed"))

	_, err := tested.createPreStartContainer(t.Context(), project, service, ctr, 0, "proj-web-pre_start-0")
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

	// createPreStartContainer calls RuntimeAPIVersion → Ping fails.
	apiClient.EXPECT().
		Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{}, errors.New("daemon unreachable"))

	_, err = s.createPreStartContainer(t.Context(), project, service, ctr, 0, "proj-web-pre_start-0")
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

	// stdout-only content: stderr frame absent so getTail falls back to stdout.
	logContent := stdcopyFrame(1, "migration failed: schema mismatch\n")

	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0))
	apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(1)).After(scan)
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(logContent)), nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	err := tested.runPreStart(t.Context(), project, service, nil)
	assert.ErrorContains(t, err, "pre_start[0]")
	// Stdout fallback: no stderr → stdout content appears in the error.
	assert.ErrorContains(t, err, "schema mismatch")
}

// TestGetHookContainerName pins the deterministic runner naming scheme the
// reconciliation plan relies on for convergence.
func TestGetHookContainerName(t *testing.T) {
	assert.Equal(t, getHookContainerName("demo", "web", 0), "demo-web-pre_start-0")
	assert.Equal(t, getHookContainerName("demo", "web", 12), "demo-web-pre_start-12")
}
