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
	"fmt"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

// TestNewLimitedErrgroup_NonPositiveIsUnlimited guards against the bug this
// helper exists to fix: errgroup.SetLimit(0) means "allow zero goroutines",
// not "unlimited". A composeService{} built without going through
// NewComposeService has maxConcurrency's Go zero-value (0), so an
// unconditional SetLimit(maxConcurrency) at any call site would silently
// hang forever instead of running.
func TestNewLimitedErrgroup_NonPositiveIsUnlimited(t *testing.T) {
	for _, maxConcurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxConcurrency=%d", maxConcurrency), func(t *testing.T) {
			eg, _ := newLimitedErrgroup(t.Context(), maxConcurrency)

			done := make(chan struct{})
			go func() {
				for range 5 {
					eg.Go(func() error { return nil })
				}
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("eg.Go blocked: maxConcurrency <= 0 must mean unlimited, not SetLimit(0) (zero goroutines allowed)")
			}
			assert.NilError(t, eg.Wait())
		})
	}
}

func swarmInfo(state swarm.LocalNodeState) client.SystemInfoResult {
	res := client.SystemInfoResult{}
	res.Info.Swarm.LocalNodeState = state
	return res
}

// TestIsSwarmEnabled_PerInstance guards that the swarm answer is cached per
// service: two services talking to different daemons must each report their
// own daemon's mode instead of sharing the first answer given in the process.
func TestIsSwarmEnabled_PerInstance(t *testing.T) {
	inactive, inactiveClient := newTestService(t)
	inactiveClient.EXPECT().Info(gomock.Any(), gomock.Any()).Times(1).
		Return(swarmInfo(swarm.LocalNodeStateInactive), nil)
	active, activeClient := newTestService(t)
	activeClient.EXPECT().Info(gomock.Any(), gomock.Any()).Times(1).
		Return(swarmInfo(swarm.LocalNodeStateActive), nil)

	for range 2 { // second round is served from each instance's own cache
		enabled, err := inactive.isSwarmEnabled(t.Context())
		assert.NilError(t, err)
		assert.Assert(t, !enabled)

		enabled, err = active.isSwarmEnabled(t.Context())
		assert.NilError(t, err)
		assert.Assert(t, enabled)
	}
}

// TestDrainTimeout guards the per-instance log drain bound and its fallback
// for a composeService not built by NewComposeService.
func TestDrainTimeout(t *testing.T) {
	svc, _ := newTestService(t)
	assert.Equal(t, svc.drainTimeout(), defaultLogStreamDrainTimeout)

	svc.logStreamDrainTimeout = time.Millisecond
	other, _ := newTestService(t)
	assert.Equal(t, svc.drainTimeout(), time.Millisecond)
	assert.Equal(t, other.drainTimeout(), defaultLogStreamDrainTimeout)

	assert.Equal(t, (&composeService{}).drainTimeout(), defaultLogStreamDrainTimeout)
}
