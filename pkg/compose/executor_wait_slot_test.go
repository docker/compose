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

package compose

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// TestExecutePlanWaitNodesHoldNoConcurrencySlot verifies that a wait node does
// not occupy a --parallel slot while it polls. The dependency below is already
// running, so the plan has no start node for its wait to depend on, and it only
// becomes healthy once a sibling container has been started: the wait for it
// and that sibling's start are independent nodes. With a single slot, a wait
// that took it would starve the start it is waiting on and only end at its
// timeout.
//
// The test does not rely on which node wins the slot: the sibling's start only
// returns once the wait has polled the dependency. Whichever node takes the
// only slot first without the fix, the other can never run, and the plan runs
// into the deadline. With the fix the wait polls without a slot, which lets the
// start return.
func TestExecutePlanWaitNodesHoldNoConcurrencySlot(t *testing.T) {
	svc, apiClient := newTestService(t)
	svc.maxConcurrency = 1

	const (
		dep       = "dep"
		depID     = "dep-id"
		siblingID = "sibling-id"
	)

	var (
		polled         = make(chan struct{})
		polledOnce     sync.Once
		siblingStarted = make(chan struct{})
	)
	apiClient.EXPECT().ContainerStart(gomock.Any(), siblingID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
			select {
			case <-polled:
				close(siblingStarted)
				return client.ContainerStartResult{}, nil
			case <-ctx.Done():
				return client.ContainerStartResult{}, ctx.Err()
			}
		})
	apiClient.EXPECT().ContainerInspect(gomock.Any(), depID, gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			polledOnce.Do(func() { close(polled) })
			health := container.Starting
			select {
			case <-siblingStarted:
				health = container.Healthy
			default:
			}
			return client.ContainerInspectResult{Container: container.InspectResponse{
				ID:   depID,
				Name: "/test-" + dep + "-1",
				State: &container.State{
					Status: container.StateRunning,
					Health: &container.Health{Status: health},
				},
				Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
			}}, nil
		}).AnyTimes()

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService[dep] = Containers{{
		ID:     depID,
		Names:  []string{"/test-" + dep + "-1"},
		Labels: map[string]string{api.ServiceLabel: dep, api.OneoffLabel: "False"},
	}}

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:" + dep,
		Name:       dep,
		Condition:  types.ServiceConditionHealthy,
	}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:sibling:1",
		Container:  &container.Summary{ID: siblingID, Names: []string{"/test-sibling-1"}},
	}, "")

	// the deadline stands in for a wait timeout: without the fix the plan only
	// ends when it expires
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	assert.NilError(t, exec.run(ctx, plan))
	assert.NilError(t, ctx.Err(), "the plan ran into the deadline instead of finishing")
}
