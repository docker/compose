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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestCollectPullTargets_VolumeOnlyImage(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"nginx": {
				Name:  "nginx",
				Image: "nginx:alpine",
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeImage,
						Source: "myorg/assets:latest",
						Target: "/srv/static",
					},
				},
			},
		},
	}

	targets := collectPullTargets(project)

	// Both the service image and the volume image should be targets.
	nginxTarget, ok := targets["nginx:alpine"]
	assert.Assert(t, ok, "expected nginx:alpine to be a pull target")
	assert.Equal(t, false, nginxTarget.isVolume)

	volTarget, ok := targets["myorg/assets:latest"]
	assert.Assert(t, ok, "expected myorg/assets:latest to be a pull target")
	assert.Equal(t, true, volTarget.isVolume)
	assert.Equal(t, "myorg/assets:latest", volTarget.service.Image)
}

func TestCollectPullTargets_ServiceWinsOverVolume(t *testing.T) {
	sharedImage := "myorg/shared:latest"

	project := &types.Project{
		Services: types.Services{
			// This service uses sharedImage both as its own image and as a volume source.
			"app": {
				Name:       "app",
				Image:      sharedImage,
				PullPolicy: types.PullPolicyNever,
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeImage,
						Source: sharedImage,
						Target: "/data",
					},
				},
			},
		},
	}

	targets := collectPullTargets(project)

	target, ok := targets[sharedImage]
	assert.Assert(t, ok, "expected shared image to be a pull target")
	assert.Equal(t, false, target.isVolume, "service entry should win over volume-only entry")
	assert.Equal(t, types.PullPolicyNever, target.service.PullPolicy)
}

func TestCollectPullTargets_BuildableVolumeImage(t *testing.T) {
	builtImage := "myorg/built:latest"

	project := &types.Project{
		Services: types.Services{
			// This service builds the image that is used as a volume source.
			"builder": {
				Name:  "builder",
				Image: builtImage,
				Build: &types.BuildConfig{Context: "."},
			},
			// This service consumes the built image as a type=image volume.
			"consumer": {
				Name: "consumer",
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeImage,
						Source: builtImage,
						Target: "/assets",
					},
				},
			},
		},
	}

	targets := collectPullTargets(project)

	target, ok := targets[builtImage]
	assert.Assert(t, ok, "expected built image to be a pull target")
	assert.Equal(t, false, target.isVolume, "builder service entry should win")

	volService := types.ServiceConfig{
		Name:  "consumer:volume 0",
		Image: builtImage,
	}
	assert.Equal(t, true, isServiceImageToBuild(volService, project.Services))

	assert.Equal(t, true, isServiceImageToBuild(target.service, project.Services))
}

func TestCollectPullTargets_Deduplication(t *testing.T) {
	sharedImage := "myorg/data:1.0"

	project := &types.Project{
		Services: types.Services{
			// Service A uses the image as its own image.
			"svc-a": {
				Name:  "svc-a",
				Image: sharedImage,
			},
			// Service B uses the same image as a volume source only.
			"svc-b": {
				Name: "svc-b",
				Volumes: []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeImage,
						Source: sharedImage,
						Target: "/mnt/data",
					},
				},
			},
		},
	}

	targets := collectPullTargets(project)

	// Only one entry for the shared image.
	target, ok := targets[sharedImage]
	assert.Assert(t, ok, "expected shared image to be a pull target")
	assert.Equal(t, false, target.isVolume, "service entry should win deduplication")
	assert.Equal(t, "svc-a", target.service.Name)
}
