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
	"io"
	"iter"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/dryrun"
	"github.com/docker/compose/v5/pkg/mocks"
)

// pushResponse is the stream of an image push, as the daemon answers it: always a 200, a failure
// being reported by a message of the stream.
type pushResponse struct {
	io.ReadCloser
}

func (pushResponse) JSONMessages(context.Context) iter.Seq2[jsonstream.Message, error] { return nil }

func (pushResponse) Wait(context.Context) error { return nil }

func pushStream(body string) client.ImagePushResponse {
	return pushResponse{io.NopCloser(strings.NewReader(body))}
}

const (
	pushedOK         = `{"status":"pushed"}` + "\n"
	missingImageFail = `{"error":"An image does not exist locally with the tag: x","errorDetail":{"message":"An image does not exist locally with the tag: x"}}` + "\n"
)

// newPushTestService serializes the pushes: the zero ConfigFile of newTestService initializes its auth
// configs lazily, which concurrent pushes would race on (a loaded config file has them initialized).
func newPushTestService(t *testing.T, events api.EventProcessor) (*composeService, *mocks.MockAPIClient) {
	t.Helper()
	return newTestService(t, WithEventProcessor(events), WithMaxConcurrency(1))
}

func pushProject() *types.Project {
	return &types.Project{
		Name: "test",
		Services: types.Services{
			"present": {
				Name:          "present",
				ContainerSpec: types.ContainerSpec{Image: "registry.example.com/app:present"},
				WorkloadSpec:  types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
			},
			"absent": {
				Name:          "absent",
				ContainerSpec: types.ContainerSpec{Image: "registry.example.com/app:absent"},
				WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{
					Context: ".",
					Tags:    []string{"registry.example.com/app:absent-extra"},
				}},
			},
		},
	}
}

func TestPushIgnoreMissingSkipsImagesNotAvailableLocally(t *testing.T) {
	events := &recordingEventProcessor{}
	svc, apiClient := newPushTestService(t, events)

	for _, tag := range []string{"registry.example.com/app:absent", "registry.example.com/app:absent-extra"} {
		apiClient.EXPECT().ImageInspect(gomock.Any(), tag).Return(client.ImageInspectResult{}, errdefs.ErrNotFound)
	}
	apiClient.EXPECT().ImageInspect(gomock.Any(), "registry.example.com/app:present").Return(client.ImageInspectResult{}, nil)
	// only the image available locally is pushed: the mock expects no other push
	apiClient.EXPECT().ImagePush(gomock.Any(), "registry.example.com/app:present", gomock.Any()).Return(pushStream(pushedOK), nil)

	err := svc.push(t.Context(), pushProject(), api.PushOptions{IgnoreMissing: true})
	assert.NilError(t, err)

	assert.Assert(t, events.contains("registry.example.com/app:present: Pushed"))
	assert.Assert(t, events.contains("registry.example.com/app:absent: Skipped"))
	assert.Assert(t, events.contains("registry.example.com/app:absent-extra: Skipped"))
	// a skipped image is never reported as being pushed
	assert.Assert(t, !events.contains("registry.example.com/app:absent: Pushing"))
}

// Without the flag, nothing changes: an image missing locally is an error, and the local images are
// not even looked up. The mock expects no ImageInspect.
func TestPushWithoutIgnoreMissingFailsOnImageNotAvailableLocally(t *testing.T) {
	svc, apiClient := newPushTestService(t, progress.Noop{})

	project := pushProject()
	delete(project.Services, "present")
	apiClient.EXPECT().ImagePush(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(pushStream(missingImageFail), nil).AnyTimes()

	err := svc.push(t.Context(), project, api.PushOptions{})
	assert.ErrorContains(t, err, "An image does not exist locally")
}

// Only a missing image is skipped: any other failure to look it up is left to the push to report.
func TestPushIgnoreMissingStillPushesWhenInspectFails(t *testing.T) {
	svc, apiClient := newPushTestService(t, progress.Noop{})

	project := pushProject()
	delete(project.Services, "absent")
	apiClient.EXPECT().ImageInspect(gomock.Any(), gomock.Any()).Return(client.ImageInspectResult{}, errors.New("daemon unavailable"))
	apiClient.EXPECT().ImagePush(gomock.Any(), "registry.example.com/app:present", gomock.Any()).Return(pushStream(pushedOK), nil)

	assert.NilError(t, svc.push(t.Context(), project, api.PushOptions{IgnoreMissing: true}))
}

// In dry-run, --ignore-missing must judge the images by what the engine really holds: the dry-run client
// delegates ImageInspect, the push of an image is faked, and nothing is ever pushed for real. The mock engine
// below expects neither an ImagePush nor a lookup of the skipped image in the registry.
func TestPushIgnoreMissingInDryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	engine := mocks.NewMockAPIClient(ctrl)
	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()

	dryRunClient, err := dryrun.NewDryRunClient(engine, cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(dryRunClient).AnyTimes()

	events := &recordingEventProcessor{}
	svc := &composeService{dockerCli: cli, dryRun: true, events: events, maxConcurrency: 1}

	for _, tag := range []string{"registry.example.com/app:absent", "registry.example.com/app:absent-extra"} {
		engine.EXPECT().ImageInspect(gomock.Any(), tag).Return(client.ImageInspectResult{}, errdefs.ErrNotFound)
	}
	engine.EXPECT().ImageInspect(gomock.Any(), "registry.example.com/app:present").Return(client.ImageInspectResult{}, nil)
	// the faked push only resolves the image it is asked to push
	engine.EXPECT().DistributionInspect(gomock.Any(), "registry.example.com/app:present", gomock.Any()).
		Return(client.DistributionInspectResult{}, nil)

	err = svc.push(t.Context(), pushProject(), api.PushOptions{IgnoreMissing: true})
	assert.NilError(t, err)

	assert.Assert(t, events.contains("registry.example.com/app:absent: Skipped"))
	assert.Assert(t, events.contains("registry.example.com/app:absent-extra: Skipped"))
	assert.Assert(t, events.contains("registry.example.com/app:present: Pushed"))
}
