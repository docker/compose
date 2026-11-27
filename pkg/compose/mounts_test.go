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
	"os"
	"path/filepath"
	"testing"

	composeloader "github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	mountTypes "github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func loadMountsProject(t *testing.T, yaml string) *composetypes.Project {
	t.Helper()
	p, err := composeloader.LoadWithContext(t.Context(), composetypes.ConfigDetails{
		ConfigFiles: []composetypes.ConfigFile{{Filename: "test", Content: []byte("name: proj\n" + yaml)}},
	}, func(options *composeloader.Options) {
		options.SkipValidation = true
		options.SkipConsistencyCheck = true
	})
	assert.NilError(t, err)
	return p
}

// A mount the service declares is made as it says, through the Mount API and never through
// the bind string, which the volumes it declares may take.
func Test_buildContainerVolumesDeclaredMounts(t *testing.T) {
	pwd, err := os.Getwd()
	assert.NilError(t, err)
	data := filepath.Join(pwd, "data")

	tests := []struct {
		name   string
		yaml   string
		mounts []mountTypes.Mount
	}{
		{
			name: "a bind creates its source only when it says so",
			yaml: `
services:
  test:
    mounts:
      - {type: bind, source: ./data, target: /data}
      - {type: bind, source: ./data, target: /created, bind: {create_src: true}}
`,
			mounts: []mountTypes.Mount{
				{Type: "bind", Source: data, Target: "/created", BindOptions: &mountTypes.BindOptions{CreateMountpoint: true}},
				{Type: "bind", Source: data, Target: "/data"},
			},
		},
		{
			name: "a bind with its propagation, recursion and access",
			yaml: `
services:
  test:
    mounts:
      - type: bind
        source: /host
        target: /data
        read_only: true
        consistency: cached
        bind:
          propagation: rshared
          recursive: readonly
`,
			mounts: []mountTypes.Mount{{
				Type: "bind", Source: "/host", Target: "/data", ReadOnly: true, Consistency: "cached",
				BindOptions: &mountTypes.BindOptions{Propagation: "rshared", ReadOnlyForceRecursive: true},
			}},
		},
		{
			name: "the recursion of a bind",
			yaml: `
services:
  test:
    mounts:
      - {type: bind, source: /a, target: /a, bind: {recursive: disabled}}
      - {type: bind, source: /b, target: /b, bind: {recursive: writable}}
`,
			mounts: []mountTypes.Mount{
				{Type: "bind", Source: "/a", Target: "/a", BindOptions: &mountTypes.BindOptions{NonRecursive: true}},
				{Type: "bind", Source: "/b", Target: "/b", BindOptions: &mountTypes.BindOptions{ReadOnlyNonRecursive: true}},
			},
		},
		{
			name: "a volume of the project is mounted by its name in the engine",
			yaml: `
services:
  test:
    mounts:
      - {type: volume, source: data, target: /data}
volumes:
  data:
    name: my_volume
`,
			mounts: []mountTypes.Mount{{Type: "volume", Source: "my_volume", Target: "/data"}},
		},
		{
			name: "the options of a volume, its driver included",
			yaml: `
services:
  test:
    mounts:
      - type: volume
        source: data
        target: /data
        volume:
          nocopy: true
          subpath: sub
          labels: {a: b}
          driver: local
          driver_opts: {o: bind}
volumes:
  data:
    name: my_volume
`,
			mounts: []mountTypes.Mount{{
				Type: "volume", Source: "my_volume", Target: "/data",
				VolumeOptions: &mountTypes.VolumeOptions{
					NoCopy: true, Subpath: "sub", Labels: map[string]string{"a": "b"},
					DriverConfig: &mountTypes.Driver{Name: "local", Options: map[string]string{"o": "bind"}},
				},
			}},
		},
		{
			name: "a tmpfs and an image",
			yaml: `
services:
  test:
    mounts:
      - {type: tmpfs, target: /scratch, tmpfs: {size: 1m, mode: 493}}
      - {type: image, source: busybox, target: /img, image: {subpath: /bin}}
`,
			mounts: []mountTypes.Mount{
				{Type: "image", Source: "busybox", Target: "/img", ImageOptions: &mountTypes.ImageOptions{Subpath: "/bin"}},
				{Type: "tmpfs", Target: "/scratch", TmpfsOptions: &mountTypes.TmpfsOptions{SizeBytes: 1048576, Mode: 0o755}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := loadMountsProject(t, tt.yaml)
			mockCtrl := gomock.NewController(t)
			mock, cli := prepareMocks(mockCtrl)
			// the image mount asks the engine for the API version it negotiated
			mock.EXPECT().Ping(gomock.Any(), gomock.Any()).AnyTimes().Return(client.PingResult{}, nil)
			mock.EXPECT().ClientVersion().AnyTimes().Return("1.50")
			s := &composeService{dockerCli: cli}
			binds, mounts, err := s.buildContainerVolumes(t.Context(), *p, p.Services["test"], nil)
			assert.NilError(t, err)
			assert.Equal(t, len(binds), 0)
			assert.DeepEqual(t, tt.mounts, mounts)
		})
	}
}

// A bind mount without a source is refused, rather than binding the current directory.
func Test_buildMountOfConfigBindWithoutSource(t *testing.T) {
	_, err := buildMountOfConfig(composetypes.Project{}, composetypes.ServiceMountConfig{Type: composetypes.VolumeTypeBind, Target: "/data"})
	assert.ErrorContains(t, err, "has no source")
}

// A mount and a volume of a service are both made, a volume added to the service after it was
// loaded (as `run --volume` does) replacing the mount of its target.
func Test_buildContainerVolumesMountsAndVolumes(t *testing.T) {
	p := loadMountsProject(t, `
services:
  test:
    volumes:
      - /host/a:/a:z
    mounts:
      - {type: tmpfs, target: /scratch}
      - {type: tmpfs, target: /b}
`)
	service := p.Services["test"]
	service.Volumes = append(service.Volumes, composetypes.ServiceVolumeConfig{Type: "bind", Source: "/host/b", Target: "/b"})

	s := &composeService{}
	binds, mounts, err := s.buildContainerVolumes(t.Context(), *p, service, nil)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"/host/a:/a:rw,z", "/host/b:/b:rw"}, binds)
	assert.DeepEqual(t, []mountTypes.Mount{{Type: "tmpfs", Target: "/scratch"}}, mounts)
}

// The anonymous volume a mount declares is the one of the container that is replaced.
func Test_buildContainerMountOptionsInheritsAnonymousVolumeMount(t *testing.T) {
	p := loadMountsProject(t, `
services:
  test:
    image: alpine
    mounts:
      - {type: volume, target: /anonymous}
      - {type: volume, source: named, target: /named}
volumes:
  named:
`)
	inherit := &container.Summary{Mounts: []container.MountPoint{
		{Type: "volume", Name: "abc123", Destination: "/anonymous", RW: true},
		{Type: "volume", Name: "other", Destination: "/named", RW: true},
	}}
	mockCtrl := gomock.NewController(t)
	mock, cli := prepareMocks(mockCtrl)
	mock.EXPECT().ImageInspect(gomock.Any(), gomock.Any()).AnyTimes().Return(client.ImageInspectResult{}, nil)
	s := &composeService{dockerCli: cli}

	mounts, err := s.buildContainerMountOptions(t.Context(), *p, p.Services["test"], inherit)
	assert.NilError(t, err)
	assert.DeepEqual(t, []mountTypes.Mount{
		{Type: "volume", Source: "abc123", Target: "/anonymous"},
		{Type: "volume", Source: "proj_named", Target: "/named"},
	}, mounts)
}

// A service that mounts a volume holds it, whichever attribute declares the mount, so that the
// volume is not removed under it and is created before it.
func Test_volumeConsumersSeesMounts(t *testing.T) {
	services := composetypes.Services{
		"viaVolumes": {Name: "viaVolumes", ContainerSpec: composetypes.ContainerSpec{
			Volumes: []composetypes.ServiceVolumeConfig{{Type: "volume", Source: "data", Target: "/d"}},
		}},
		"viaMounts": {Name: "viaMounts", ContainerSpec: composetypes.ContainerSpec{
			Mounts: []composetypes.ServiceMountConfig{{Type: "volume", Source: "data", Target: "/d"}},
		}},
		"viaBind": {Name: "viaBind", ContainerSpec: composetypes.ContainerSpec{
			Mounts: []composetypes.ServiceMountConfig{{Type: "bind", Source: "data", Target: "/d"}},
		}},
		"inherits": {Name: "inherits", ContainerSpec: composetypes.ContainerSpec{VolumesFrom: []string{"viaMounts"}}},
	}
	assert.DeepEqual(t, []string{"inherits", "viaMounts", "viaVolumes"}, volumeConsumers(services, "data"))
}

func Test_imageVolumeSourcesAndResolution(t *testing.T) {
	service := composetypes.ServiceConfig{Name: "app", ContainerSpec: composetypes.ContainerSpec{
		Volumes: []composetypes.ServiceVolumeConfig{{Type: "image", Source: "one", Target: "/one"}},
		Mounts: []composetypes.ServiceMountConfig{
			{Type: "image", Source: "two", Target: "/two"},
			{Type: "volume", Source: "three", Target: "/three"},
		},
	}}
	assert.DeepEqual(t, []string{"one", "two"}, imageVolumeSources(service))

	images := map[string]api.ImageSummary{
		"one": {ID: "sha256:1"},
		"two": {ID: "sha256:2"},
	}
	resolveImageVolumes(&service, images, "proj")
	assert.Equal(t, service.CustomLabels[api.ImageVolumeDigestLabel], "/one=sha256:1,/two=sha256:2")
}

func Test_withOwnMutableFieldsClonesMounts(t *testing.T) {
	mounts := []composetypes.ServiceMountConfig{{Type: "image", Source: "img", Target: "/i"}}
	own := withOwnMutableFields(composetypes.ServiceConfig{ContainerSpec: composetypes.ContainerSpec{Mounts: mounts}})
	own.Mounts[0].Source = "changed"
	assert.Equal(t, mounts[0].Source, "img")
}
