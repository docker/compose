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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

const testDigest = "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

func TestResolveImageDigests(t *testing.T) {
	// distinct digests per resolved reference, so attaching a digest to the wrong image would fail
	const (
		serviceDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hookDigest    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		volumeDigest  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()

	model := map[string]any{
		"services": map[string]any{
			"test": map[string]any{
				"image": "nginx:latest",
				"pre_start": []any{
					map[string]any{"command": "echo hello", "image": "hookimage:latest"},
					// hook running in the service container: no image to resolve
					map[string]any{"command": "echo hello"},
				},
				"volumes": []any{
					map[string]any{"type": "image", "source": "someimage:latest", "target": "/data"},
					// already digested: must NOT trigger any registry call and must be kept as-is
					map[string]any{"type": "image", "source": "docker.io/library/pinned@" + testDigest, "target": "/pinned"},
					// source referencing another service: locally built image, must be kept as-is
					map[string]any{"type": "image", "source": "builder", "target": "/built"},
					map[string]any{"type": "bind", "source": "/host", "target": "/bind"},
					"./data:/short",
				},
			},
			"builder": map[string]any{
				"image": "docker.io/library/pinned@" + testDigest,
			},
			// service without image: its image volumes must still be pinned, and
			// "someimage:latest" being also used by "test" must be resolved only once
			"data": map[string]any{
				"volumes": []any{
					map[string]any{"type": "image", "source": "someimage:latest", "target": "/data"},
				},
			},
		},
	}

	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/nginx:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: ocispec.Descriptor{Digest: serviceDigest}},
		}, nil)
	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/someimage:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: ocispec.Descriptor{Digest: volumeDigest}},
		}, nil)
	apiClient.EXPECT().DistributionInspect(gomock.Any(), "docker.io/library/hookimage:latest", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{Descriptor: ocispec.Descriptor{Digest: hookDigest}},
		}, nil)

	err := resolveImageDigests(t.Context(), cli, model)
	assert.NilError(t, err)

	services := model["services"].(map[string]any)
	service := services["test"].(map[string]any)
	assert.Equal(t, service["image"], "docker.io/library/nginx:latest@"+serviceDigest)
	hooks := service["pre_start"].([]any)
	assert.Equal(t, hooks[0].(map[string]any)["image"], "docker.io/library/hookimage:latest@"+hookDigest)
	_, hasImage := hooks[1].(map[string]any)["image"]
	assert.Assert(t, !hasImage)
	volumes := service["volumes"].([]any)
	assert.Equal(t, volumes[0].(map[string]any)["source"], "docker.io/library/someimage:latest@"+volumeDigest)
	assert.Equal(t, volumes[1].(map[string]any)["source"], "docker.io/library/pinned@"+testDigest)
	assert.Equal(t, volumes[2].(map[string]any)["source"], "builder")
	assert.Equal(t, volumes[3].(map[string]any)["source"], "/host")
	assert.Equal(t, volumes[4], "./data:/short")
	assert.Equal(t, services["builder"].(map[string]any)["image"], "docker.io/library/pinned@"+testDigest)
	dataVolumes := services["data"].(map[string]any)["volumes"].([]any)
	assert.Equal(t, dataVolumes[0].(map[string]any)["source"], "docker.io/library/someimage:latest@"+volumeDigest)
}

func TestResolveImageDigestsWithoutServices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(mocks.NewMockAPIClient(mockCtrl)).AnyTimes()
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()

	// top-level services is optional in the compose spec, and unlike the typed
	// project the raw model gets no empty services map from normalization
	model := map[string]any{
		"networks": map[string]any{"foo": nil},
	}

	err := resolveImageDigests(t.Context(), cli, model)
	assert.NilError(t, err)
}

func TestImagesOnly(t *testing.T) {
	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"test": types.ServiceConfig{
				Name:    "test",
				Image:   "docker.io/library/nginx@" + testDigest,
				Command: types.ShellCommand{"echo", "hello"},
				// hooks can't be overridden element-wise on merge, so the lock must not carry them
				PreStart: []types.ServiceHook{{Image: "docker.io/library/hookimage@" + testDigest}},
				Volumes: []types.ServiceVolumeConfig{
					{Type: types.VolumeTypeImage, Source: "docker.io/library/someimage@" + testDigest, Target: "/data"},
					{Type: types.VolumeTypeBind, Source: "/host", Target: "/bind"},
				},
			},
		},
		Networks: types.Networks{"default": types.NetworkConfig{}},
	}

	locked := imagesOnly(project)

	assert.DeepEqual(t, locked, &types.Project{
		Services: types.Services{
			"test": types.ServiceConfig{
				Image: "docker.io/library/nginx@" + testDigest,
				Volumes: []types.ServiceVolumeConfig{
					{Type: types.VolumeTypeImage, Source: "docker.io/library/someimage@" + testDigest, Target: "/data"},
				},
			},
		},
	})
}

func TestWarnHooksNotLockable(t *testing.T) {
	hook := logrustest.NewGlobal()
	logrus.SetOutput(io.Discard)
	defer func() {
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
		logrus.SetOutput(os.Stderr)
	}()

	warnHooksNotLockable(&types.Project{
		Services: types.Services{
			"with-hook-image": types.ServiceConfig{PreStart: []types.ServiceHook{{Image: "alpine:latest"}}},
			"inline-hook":     types.ServiceConfig{PreStart: []types.ServiceHook{{Command: types.ShellCommand{"echo"}}}},
			"without-hook":    types.ServiceConfig{},
		},
	})

	assert.Equal(t, len(hook.Entries), 1)
	assert.Assert(t, strings.Contains(hook.Entries[0].Message, `service "with-hook-image"`))
}

func TestWarnModelHooksNotLockable(t *testing.T) {
	hook := logrustest.NewGlobal()
	logrus.SetOutput(io.Discard)
	defer func() {
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
		logrus.SetOutput(os.Stderr)
	}()

	warnModelHooksNotLockable(map[string]any{
		"services": map[string]any{
			"with-hook-image": map[string]any{
				"pre_start": []any{map[string]any{"image": "alpine:latest"}},
			},
			"inline-hook": map[string]any{
				"pre_start": []any{map[string]any{"command": "echo"}},
			},
			"without-hook": map[string]any{"image": "nginx"},
		},
	})

	assert.Equal(t, len(hook.Entries), 1)
	assert.Assert(t, strings.Contains(hook.Entries[0].Message, `service "with-hook-image"`))
}

func TestLockModel(t *testing.T) {
	model := map[string]any{
		"name": "test",
		"services": map[string]any{
			"a": map[string]any{
				"image":   "docker.io/library/nginx@" + testDigest,
				"command": "echo hello",
				// hooks can't be overridden element-wise on merge, so the lock must not carry them
				"pre_start": []any{
					map[string]any{"image": "docker.io/library/hookimage@" + testDigest},
				},
				"volumes": []any{
					map[string]any{"type": "image", "source": "docker.io/library/someimage@" + testDigest, "target": "/data"},
					map[string]any{"type": "bind", "source": "/host", "target": "/bind"},
				},
			},
			"b": map[string]any{
				"image": "docker.io/library/alpine@" + testDigest,
				"volumes": []any{
					map[string]any{"type": "bind", "source": "/host", "target": "/bind"},
				},
			},
		},
		"networks": map[string]any{"default": nil},
	}

	lockModel(model)

	assert.DeepEqual(t, model, map[string]any{
		"services": map[string]any{
			"a": map[string]any{
				"image": "docker.io/library/nginx@" + testDigest,
				// volumes keeps its []any raw-model type after filtering
				"volumes": []any{
					map[string]any{"type": "image", "source": "docker.io/library/someimage@" + testDigest, "target": "/data"},
				},
			},
			"b": map[string]any{
				"image": "docker.io/library/alpine@" + testDigest,
			},
		},
	})
}
