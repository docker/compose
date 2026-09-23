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
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// waitTestService builds a composeService whose ContainerList answers depend
// on the All flag: running containers first, the full set on the fallback
// listing. Call counts let tests assert which listings actually happened.
func waitTestService(t *testing.T, running, all []container.Summary) (api.Compose, *mocks.MockAPIClient, *int, *int) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	apiClient, cli := prepareMocks(mockCtrl)
	runningCalls, allCalls := new(int), new(int)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
			if opts.All {
				*allCalls++
				return client.ContainerListResult{Items: all}, nil
			}
			*runningCalls++
			return client.ContainerListResult{Items: running}, nil
		}).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	return tested, apiClient, runningCalls, allCalls
}

// TestWait_AlreadyExitedTarget locks the race fix: a target that exited
// before wait listed the project is a SATISFIED wait — its recorded exit
// code is returned immediately — not a "no containers" error.
func TestWait_AlreadyExitedTarget(t *testing.T) {
	exited := container.Summary{
		ID: "c-exited", State: container.StateExited,
		Labels: map[string]string{api.ServiceLabel: "faster", api.ContainerNumberLabel: "1"},
	}
	tested, apiClient, runningCalls, allCalls := waitTestService(t, nil, []container.Summary{exited})

	apiClient.EXPECT().ContainerWait(gomock.Any(), "c-exited", gomock.Any()).
		Return(waitResultExit(7))

	code, err := tested.Wait(t.Context(), "proj", api.WaitOptions{Services: []string{"faster"}})
	assert.NilError(t, err)
	assert.Equal(t, code, int64(7))
	assert.Equal(t, *runningCalls, 1)
	assert.Equal(t, *allCalls, 1)
}

// TestWait_NoContainersAtAll: when neither listing finds a matching
// container, the invocation is wrong and still errors.
func TestWait_NoContainersAtAll(t *testing.T) {
	tested, _, runningCalls, allCalls := waitTestService(t, nil, nil)

	_, err := tested.Wait(t.Context(), "proj", api.WaitOptions{})
	assert.ErrorContains(t, err, `no containers for project "proj"`)
	assert.Equal(t, *runningCalls, 1)
	assert.Equal(t, *allCalls, 1)
}

// TestWait_RunningContainersSkipFallback: with a running container to
// observe, the fallback listing never runs — a stale exited one-off cannot
// short-circuit the wait.
func TestWait_RunningContainersSkipFallback(t *testing.T) {
	running := container.Summary{
		ID: "c-run", State: container.StateRunning,
		Labels: map[string]string{api.ServiceLabel: "slower", api.ContainerNumberLabel: "1"},
	}
	tested, apiClient, runningCalls, allCalls := waitTestService(t, []container.Summary{running}, nil)

	apiClient.EXPECT().ContainerWait(gomock.Any(), "c-run", gomock.Any()).
		Return(waitResultExit(0))

	code, err := tested.Wait(t.Context(), "proj", api.WaitOptions{})
	assert.NilError(t, err)
	assert.Equal(t, code, int64(0))
	assert.Equal(t, *runningCalls, 1)
	assert.Equal(t, *allCalls, 0)
}
