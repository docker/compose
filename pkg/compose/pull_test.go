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
	"io"
	"iter"
	"sort"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	imagetypes "github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

type testImagePullResponse struct {
	io.ReadCloser
}

func (r testImagePullResponse) JSONMessages(context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {}
}

func (r testImagePullResponse) Wait(context.Context) error {
	return nil
}

func TestPullServiceImageUsesRunnableContentDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	dockerAPI, cli := prepareMocks(mockCtrl)
	cli.EXPECT().ConfigFile().Return(configfile.New(""))
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	dockerAPI.EXPECT().ImagePull(gomock.Any(), "foo:1", gomock.Any()).Return(testImagePullResponse{
		ReadCloser: io.NopCloser(strings.NewReader("")),
	}, nil)
	dockerAPI.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.48"}, nil)
	dockerAPI.EXPECT().ClientVersion().Return("1.48")
	dockerAPI.EXPECT().ImageInspect(gomock.Any(), "foo:1", gomock.Any()).Return(client.ImageInspectResult{
		InspectResponse: imagetypes.InspectResponse{
			ID: "sha256:index",
			Manifests: []imagetypes.ManifestSummary{{
				ID:        "sha256:image",
				Kind:      imagetypes.ManifestKindImage,
				Available: true,
				ImageData: &imagetypes.ImageProperties{
					Platform: specs.Platform{OS: "linux", Architecture: "amd64"},
				},
			}},
		},
	}, nil)

	id, err := tested.(*composeService).pullServiceImage(t.Context(), types.ServiceConfig{Image: "foo:1"}, true, "")
	assert.NilError(t, err)
	assert.Equal(t, id, "sha256:image")
}

// scheduledHookImages runs addPreStartHookPulls and returns the hook image
// references it scheduled for pull, sorted for deterministic assertions.
func scheduledHookImages(t *testing.T, project *types.Project, present map[string]api.ImageSummary) []string {
	t.Helper()
	needPull := map[string]types.ServiceConfig{}
	scheduled := map[string]bool{}
	// Seed scheduled with the service images, as pullRequiredImages does before
	// calling addPreStartHookPulls.
	for _, service := range project.Services {
		scheduled[service.Image] = true
	}
	addPreStartHookPulls(project, present, needPull, scheduled)
	var images []string
	for _, s := range needPull {
		images = append(images, s.Image)
	}
	sort.Strings(images)
	return images
}

func serviceWithHook(name, image, policy string) types.ServiceConfig {
	return types.ServiceConfig{
		Name:       name,
		Image:      image,
		PullPolicy: policy,
		PreStart:   []types.ServiceHook{{Image: "init:latest"}},
	}
}

// TestAddPreStartHookPulls_AlwaysForcesPresentHook covers the docker-agent
// finding: `pull_policy: always` must re-pull a hook image even when it is
// already present locally, mirroring how the service image is force-pulled.
func TestAddPreStartHookPulls_AlwaysForcesPresentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyAlways)},
	}
	present := map[string]api.ImageSummary{"init:latest": {ID: "sha256:present"}}

	assert.DeepEqual(t, scheduledHookImages(t, project, present), []string{"init:latest"})
}

// TestAddPreStartHookPulls_MissingPolicySkipsPresentHook verifies a present hook
// image is not re-pulled under the default (pull-if-missing) behavior.
func TestAddPreStartHookPulls_MissingPolicySkipsPresentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing)},
	}
	present := map[string]api.ImageSummary{"init:latest": {ID: "sha256:present"}}

	assert.Equal(t, len(scheduledHookImages(t, project, present)), 0)
}

// TestAddPreStartHookPulls_MissingPolicyPullsAbsentHook verifies an absent hook
// image is pulled under the default policy.
func TestAddPreStartHookPulls_MissingPolicyPullsAbsentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing)},
	}

	assert.DeepEqual(t, scheduledHookImages(t, project, map[string]api.ImageSummary{}), []string{"init:latest"})
}

// TestAddPreStartHookPulls_NeverSkips verifies `pull_policy: never` never
// schedules a hook pull, even for an absent image.
func TestAddPreStartHookPulls_NeverSkips(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyNever)},
	}

	assert.Equal(t, len(scheduledHookImages(t, project, map[string]api.ImageSummary{})), 0)
}

// TestAddPreStartHookPulls_DedupsSharedHookImage verifies a hook image shared by
// several services is scheduled at most once.
func TestAddPreStartHookPulls_DedupsSharedHookImage(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing),
			"api": serviceWithHook("api", "api:latest", types.PullPolicyMissing),
		},
	}

	assert.DeepEqual(t, scheduledHookImages(t, project, map[string]api.ImageSummary{}), []string{"init:latest"})
}
