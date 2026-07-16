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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/compose/v5/pkg/mocks"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

const (
	configDigestServiceName      = "app"
	configDigestServiceImage     = "busybox:latest"
	configDigestServiceRef       = "docker.io/library/busybox:latest"
	configDigestServicePinned    = "docker.io/library/busybox:latest@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	configDigestServiceDigest    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	configDigestVolumeType       = "image"
	configDigestVolumeSource     = "alpine:latest"
	configDigestVolumeRef        = "docker.io/library/alpine:latest"
	configDigestVolumePinned     = "docker.io/library/alpine:latest@sha256:2222222222222222222222222222222222222222222222222222222222222222"
	configDigestVolumeDigest     = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	configDigestVolumeTarget     = "/test_mount"
	configDigestModelServicesKey = "services"
	configDigestModelImageKey    = "image"
	configDigestModelVolumesKey  = "volumes"
	configDigestModelTypeKey     = "type"
	configDigestModelSourceKey   = "source"
	configDigestModelTargetKey   = "target"
	configDigestModelCommandKey  = "command"
	configDigestBindVolumeType   = "bind"
	configDigestBindVolumeSource = "./data"
	configDigestBindVolumeTarget = "/data"
)

func TestResolveImageDigestsPinsImageVolumeSources(t *testing.T) {
	ctrl := gomock.NewController(t)
	dockerCli := mocks.NewMockCli(ctrl)
	apiClient := mocks.NewMockAPIClient(ctrl)

	dockerCli.EXPECT().ConfigFile().Return(configfile.New(""))
	dockerCli.EXPECT().Client().Return(apiClient)
	apiClient.EXPECT().
		DistributionInspect(gomock.Any(), configDigestServiceRef, gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{
				Descriptor: ocispec.Descriptor{Digest: configDigestServiceDigest},
			},
		}, nil)
	apiClient.EXPECT().
		DistributionInspect(gomock.Any(), configDigestVolumeRef, gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registry.DistributionInspect{
				Descriptor: ocispec.Descriptor{Digest: configDigestVolumeDigest},
			},
		}, nil)

	volume := map[string]any{
		configDigestModelTypeKey:   configDigestVolumeType,
		configDigestModelSourceKey: configDigestVolumeSource,
		configDigestModelTargetKey: configDigestVolumeTarget,
	}
	service := map[string]any{
		configDigestModelImageKey:   configDigestServiceImage,
		configDigestModelVolumesKey: []any{volume},
	}
	model := map[string]any{
		configDigestModelServicesKey: map[string]any{
			configDigestServiceName: service,
		},
	}

	err := resolveImageDigests(context.Background(), dockerCli, model)
	assert.NilError(t, err)

	assert.Equal(t, service[configDigestModelImageKey], configDigestServicePinned)
	assert.Equal(t, volume[configDigestModelSourceKey], configDigestVolumePinned)
}

func TestImagesOnlyKeepsImageVolumeSources(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			configDigestServiceName: {
				Image: configDigestServicePinned,
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeImage,
						Source: configDigestVolumePinned,
						Target: configDigestVolumeTarget,
					},
					{
						Type:   types.VolumeTypeBind,
						Source: configDigestBindVolumeSource,
						Target: configDigestBindVolumeTarget,
					},
				},
			},
		},
	}

	locked := imagesOnly(project)
	service := locked.Services[configDigestServiceName]

	assert.Equal(t, service.Image, configDigestServicePinned)
	assert.Equal(t, len(service.Volumes), 1)
	assert.Equal(t, service.Volumes[0].Type, types.VolumeTypeImage)
	assert.Equal(t, service.Volumes[0].Source, configDigestVolumePinned)
	assert.Equal(t, service.Volumes[0].Target, configDigestVolumeTarget)
}

func TestLockModelOnlyKeepsImageVolumeSources(t *testing.T) {
	imageVolume := map[string]any{
		configDigestModelTypeKey:   configDigestVolumeType,
		configDigestModelSourceKey: configDigestVolumePinned,
		configDigestModelTargetKey: configDigestVolumeTarget,
	}
	bindVolume := map[string]any{
		configDigestModelTypeKey:   configDigestBindVolumeType,
		configDigestModelSourceKey: configDigestBindVolumeSource,
		configDigestModelTargetKey: configDigestBindVolumeTarget,
	}
	service := map[string]any{
		configDigestModelImageKey:   configDigestServicePinned,
		configDigestModelCommandKey: []any{"true"},
		configDigestModelVolumesKey: []any{imageVolume, bindVolume},
	}
	model := map[string]any{
		configDigestModelServicesKey: map[string]any{
			configDigestServiceName: service,
		},
	}

	lockModelOnly(model)

	assert.Equal(t, len(service), 2)
	assert.Equal(t, service[configDigestModelImageKey], configDigestServicePinned)
	volumes := service[configDigestModelVolumesKey].([]any)
	assert.Equal(t, len(volumes), 1)
	assert.DeepEqual(t, volumes[0], imageVolume)
}
