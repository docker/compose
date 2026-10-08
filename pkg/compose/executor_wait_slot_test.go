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
	"fmt"
	"sync/atomic"
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
// not occupy a --parallel slot while it polls. Each pair below is a dependency
// that is already running, so the plan has no start node for its wait to
// depend on, and that only becomes healthy once a sibling container has been
// started: the wait for it and that sibling's start are independent nodes. With a
// single slot, a wait that took it first would starve the start it is waiting
// on and only end at its timeout. Several pairs are used so the nodes racing
// for the slot make that outcome all but certain without the fix: it takes
// every start winning the slot before any wait does.
func TestExecutePlanWaitNodesHoldNoConcurrencySlot(t *testing.T) {
	const pairs = 5

	svc, apiClient, _ := newStartPhaseTestService(t)
	svc.maxConcurrency = 1

	plan := &Plan{}
	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	for i := range pairs {
		dep := fmt.Sprintf("dep%d", i)
		depID := dep + "-id"
		siblingID := fmt.Sprintf("sibling%d-id", i)

		var siblingStarted atomic.Bool
		apiClient.EXPECT().ContainerStart(gomock.Any(), siblingID, gomock.Any()).
			DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
				siblingStarted.Store(true)
				return client.ContainerStartResult{}, nil
			})
		apiClient.EXPECT().ContainerInspect(gomock.Any(), depID, gomock.Any()).
			DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				health := container.Starting
				if siblingStarted.Load() {
					health = container.Healthy
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

		exec.containersByService[dep] = Containers{{
			ID:     depID,
			Names:  []string{"/test-" + dep + "-1"},
			Labels: map[string]string{api.ServiceLabel: dep, api.OneoffLabel: "False"},
		}}
		plan.addNode(Operation{
			Type:       OpWaitCondition,
			ResourceID: "wait:" + dep,
			Name:       dep,
			Condition:  types.ServiceConditionHealthy,
		}, "")
		plan.addNode(Operation{
			Type:       OpStartContainer,
			ResourceID: fmt.Sprintf("service:sibling%d:1", i),
			Container:  &container.Summary{ID: siblingID, Names: []string{fmt.Sprintf("/test-sibling%d-1", i)}},
		}, "")
	}

	// the deadline stands in for a wait timeout: without the fix the starved
	// waits only end when it expires
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	assert.NilError(t, exec.run(ctx, plan))
	assert.NilError(t, ctx.Err(), "the plan ran into the deadline instead of finishing")
}
