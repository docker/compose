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
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// bracketRecorder records progress events together with the Start/Done
// bracket boundaries, to tell which events were emitted inside an operation.
type bracketRecorder struct {
	mu      sync.Mutex
	entries []string
}

func (r *bracketRecorder) record(entry string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
}

func (r *bracketRecorder) Start(_ context.Context, operation string) { r.record("start " + operation) }

func (r *bracketRecorder) Done(operation string, _ bool) { r.record("done " + operation) }

func (r *bracketRecorder) On(events ...api.Resource) {
	for _, e := range events {
		r.record(e.ID + " " + e.Text)
	}
}

// TestPrepareRunPullStaysInsideRunOperation guards the progress rendering of
// `compose run` when the target service's image has to be pulled: the pull
// events must be emitted between the "run" operation's Start and Done, as the
// TTY writer only draws its compact block there and degrades to one plain line
// per event (including per-layer ones) for anything outside.
func TestPrepareRunPullStaysInsideRunOperation(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiClient, cli := prepareMocks(mockCtrl)
	rec := &bracketRecorder{}
	svc, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	tested := svc.(*composeService)
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.48"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.48").AnyTimes()
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()
	cli.EXPECT().In().Return(streams.NewIn(os.Stdin)).AnyTimes()

	apiClient.EXPECT().ImageInspect(gomock.Any(), "foo:1", gomock.Any()).
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:foo"}}, nil).AnyTimes()
	var pulled atomic.Bool
	apiClient.EXPECT().ImagePull(gomock.Any(), "foo:1", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ImagePullOptions) (client.ImagePullResponse, error) {
			pulled.Store(true)
			return fakePullResponse{}, nil
		})
	// stop once the image is available: only the ordering of the pull
	// relative to the operation bracket matters here
	stop := errors.New("stop after pull")
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
			if pulled.Load() {
				return client.ContainerListResult{}, stop
			}
			return client.ContainerListResult{}, nil
		}).AnyTimes()

	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil).AnyTimes()
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{}, nil).AnyTimes()

	project := &types.Project{
		Name: "p",
		Services: types.Services{
			"t": {
				Name: "t",
				ContainerSpec: types.ContainerSpec{
					Image:        "foo:1",
					PullPolicy:   types.PullPolicyAlways,
					CustomLabels: types.Labels{},
				},
			},
		},
	}
	_, err = tested.prepareRun(t.Context(), project, api.RunOptions{Service: "t"})
	assert.ErrorIs(t, err, stop)

	pulling := -1
	for i, entry := range rec.entries {
		if entry == "Image foo:1 "+api.StatusPulling {
			pulling = i
		}
	}
	assert.Assert(t, pulling >= 0, "no pull event recorded: %v", rec.entries)
	start, done := -1, -1
	for i, entry := range rec.entries {
		switch entry {
		case "start run":
			start = i
		case "done run":
			done = i
		}
	}
	assert.Assert(t, start >= 0 && start < pulling, "pull started before the run operation: %v", rec.entries)
	assert.Assert(t, done > pulling, "run operation closed before the pull: %v", rec.entries)
}
