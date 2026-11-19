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
	"testing"
	"time"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

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

// TestWithDryRunKeepsLogLevel guards that switching to the dry-run client
// does not reset the log level: re-initializing the Docker CLI with fresh
// client options would otherwise drop --verbose back to info, hiding every
// debug log of the dry-run operation.
func TestWithDryRunKeepsLogLevel(t *testing.T) {
	previous := logrus.GetLevel()
	t.Cleanup(func() { logrus.SetLevel(previous) })
	logrus.SetLevel(logrus.TraceLevel)

	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(mocks.NewMockAPIClient(mockCtrl)).AnyTimes()
	cli.EXPECT().CurrentContext().Return("default").AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()

	assert.NilError(t, WithDryRun(&composeService{dockerCli: cli}))
	assert.Equal(t, logrus.GetLevel(), logrus.TraceLevel)
}

// TestEngineAPIIsReachedWithoutDockerCLI guards that the engine API is reached
// through the engine alone: a service handed an engine runs its operations and
// releases it on Close without ever asking the Docker CLI, which a mock with no
// expectation would fail on.
func TestEngineAPIIsReachedWithoutDockerCLI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	svc := &composeService{
		engine:    apiClient,
		dockerCli: mocks.NewMockCli(mockCtrl),
	}

	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{
		Items: []volume.Volume{{
			Name:   "proj_data",
			Driver: "local",
			Labels: map[string]string{api.VolumeLabel: "data"},
		}},
	}, nil)
	volumes, err := svc.actualVolumes(t.Context(), "proj")
	assert.NilError(t, err)
	assert.Equal(t, volumes["data"].Name, "proj_data")

	apiClient.EXPECT().Close().Return(nil)
	assert.NilError(t, svc.Close())
}

// TestWithDryRunRefusesInjectedEngine guards that a dry run is never silently
// skipped: the dry-run client only wraps the client of the Docker CLI, so a
// service holding an engine of its own cannot honor it and says so.
func TestWithDryRunRefusesInjectedEngine(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	svc := &composeService{engine: mocks.NewMockAPIClient(mockCtrl)}

	err := WithDryRun(svc)
	assert.ErrorContains(t, err, "injected engine")
	assert.Assert(t, !svc.dryRun)
}
