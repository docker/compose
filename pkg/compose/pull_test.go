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
	"io"
	"iter"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	composeapi "github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestPullIncludesPreStartHookImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "hooktest",
		Services: types.Services{
			"demo": {
				Name:       "demo",
				Image:      "alpine:3.20",
				PullPolicy: types.PullPolicyMissing,
				PreStart: []types.ServiceHook{
					{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "hook"}},
				},
			},
		},
	}

	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine:3.20").
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:service"}}, nil)
	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine:3.19").
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound.WithMessage("missing hook image"))
	expectSuccessfulImagePull(apiClient, "alpine:3.19", "sha256:hook")

	err = tested.(*composeService).pull(t.Context(), project, composeapi.PullOptions{})
	assert.NilError(t, err)
}

func TestPullRequiredImagesIncludesPreStartHookImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "hooktest",
		Services: types.Services{
			"demo": {
				Name:       "demo",
				Image:      "alpine:3.20",
				PullPolicy: types.PullPolicyMissing,
				PreStart: []types.ServiceHook{
					{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "hook"}},
				},
			},
		},
	}
	images := map[string]composeapi.ImageSummary{
		"alpine:3.20": {ID: "sha256:service"},
	}

	expectSuccessfulImagePull(apiClient, "alpine:3.19", "sha256:hook")

	err = tested.(*composeService).pullRequiredImages(t.Context(), project, images, true)
	assert.NilError(t, err)
	assert.Equal(t, images["alpine:3.19"].ID, "sha256:hook")
}

func expectSuccessfulImagePull(apiClient *mocks.MockAPIClient, ref string, id string) {
	apiClient.EXPECT().ImagePull(gomock.Any(), ref, gomock.Any()).
		Return(testImagePullResponse{ReadCloser: io.NopCloser(strings.NewReader(""))}, nil)
	apiClient.EXPECT().ImageInspect(gomock.Any(), ref).
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: id}}, nil)
}

type testImagePullResponse struct {
	io.ReadCloser
}

func (r testImagePullResponse) JSONMessages(ctx context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {}
}

func (r testImagePullResponse) Wait(ctx context.Context) error {
	return nil
}
