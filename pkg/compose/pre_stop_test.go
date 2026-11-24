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
	"errors"
	"fmt"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// A pre_stop hook acts on the process of a container: stopping or restarting
// one that has none (docker/compose#14545) must neither run the hooks nor fail
// for want of them. No ExecCreate expectation is set in the tests that skip the
// hooks: per newStartTestService, gomock fails the test if one still runs.

func preStopService(hooks ...string) types.ServiceConfig {
	service := types.ServiceConfig{Name: "web"}
	for _, h := range hooks {
		service.PreStop = append(service.PreStop, types.ServiceHook{Command: types.ShellCommand{h}})
	}
	return service
}

// errNotRunning is what the engine answers an exec in a container that has no
// process to run it.
func errNotRunning(id string) error {
	return fmt.Errorf("container %s is not running: %w", id, errdefs.ErrConflict)
}

func TestPreStopHooksAreSkippedForAContainerThatDoesNotRun(t *testing.T) {
	states := []container.ContainerState{container.StateExited, container.StatePaused, container.StateCreated, container.StateRestarting}
	for _, state := range states {
		t.Run("restart/"+string(state), func(t *testing.T) {
			svc, apiClient, _ := newStartTestService(t)
			ctr := serviceContainer("web")
			ctr.State = state
			apiClient.EXPECT().ContainerRestart(gomock.Any(), ctr.ID, gomock.Any()).
				Return(client.ContainerRestartResult{}, nil)

			err := svc.restartContainer(t.Context(), preStopService("quiesce", "flush"), ctr, api.RestartOptions{})
			assert.NilError(t, err)
		})
		t.Run("stop/"+string(state), func(t *testing.T) {
			svc, apiClient, rec := newStartTestService(t)
			ctr := serviceContainer("web")
			ctr.State = state
			service := preStopService("quiesce", "flush")
			apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
				Return(client.ContainerStopResult{}, nil)

			err := svc.stopContainer(t.Context(), &service, ctr, nil, nil)
			assert.NilError(t, err)
			assert.Assert(t, rec.contains("Container prj-web-1: Stopped"), "the row ends: %v", rec.summary())
		})
	}
}

// The state of a listed container may be out of date: the engine's refusal is
// then what says there is no process, and the remaining hooks are skipped.
func TestPreStopHooksStopAtTheEnginesRefusalButTheStopGoesOn(t *testing.T) {
	t.Run("restart", func(t *testing.T) {
		svc, apiClient, _ := newStartTestService(t)
		ctr := serviceContainer("web")
		apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ExecCreateResult{}, errNotRunning(ctr.ID)).Times(1)
		apiClient.EXPECT().ContainerRestart(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ContainerRestartResult{}, nil)

		err := svc.restartContainer(t.Context(), preStopService("quiesce", "flush"), ctr, api.RestartOptions{})
		assert.NilError(t, err)
	})
	t.Run("stop", func(t *testing.T) {
		svc, apiClient, _ := newStartTestService(t)
		ctr := serviceContainer("web")
		service := preStopService("quiesce", "flush")
		apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ExecCreateResult{}, errNotRunning(ctr.ID)).Times(1)
		apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ContainerStopResult{}, nil)

		err := svc.stopContainer(t.Context(), &service, ctr, nil, nil)
		assert.NilError(t, err)
	})
}

// Skipping is for a container with no process, not for a hook that fails: its
// failure is the user's to know, and the stop or the restart does not happen.
func TestAFailingPreStopHookStillFailsTheStopAndTheRestart(t *testing.T) {
	boom := errors.New("exec failed")
	t.Run("restart", func(t *testing.T) {
		svc, apiClient, _ := newStartTestService(t)
		ctr := serviceContainer("web")
		apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ExecCreateResult{}, boom)

		err := svc.restartContainer(t.Context(), preStopService("quiesce"), ctr, api.RestartOptions{})
		assert.ErrorIs(t, err, boom)
	})
	t.Run("stop", func(t *testing.T) {
		svc, apiClient, _ := newStartTestService(t)
		ctr := serviceContainer("web")
		service := preStopService("quiesce")
		apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
			Return(client.ExecCreateResult{}, boom)

		err := svc.stopContainer(t.Context(), &service, ctr, nil, nil)
		assert.ErrorIs(t, err, boom)
	})
}

// A container removed since it was listed has nothing left to stop.
func TestStopContainerRemovedMeanwhileIsNotAnError(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	ctr := serviceContainer("web")
	service := preStopService("quiesce")
	apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{}, fmt.Errorf("no such container: %w", errdefs.ErrNotFound))

	err := svc.stopContainer(t.Context(), &service, ctr, nil, nil)
	assert.NilError(t, err)
}
