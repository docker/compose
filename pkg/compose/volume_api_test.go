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
	mountTypes "github.com/moby/moby/api/types/mount"
	"gotest.tools/v3/assert"
)

// How the engine takes a volume a service declares: the bind string when it says all the volume says,
// the Mount API when it can't, and an error when the engine can't take the volume either way.
func TestVolumeNeedsMountAPI(t *testing.T) {
	bind := func(b composetypes.ServiceVolumeBind) composetypes.ServiceVolumeConfig {
		return composetypes.ServiceVolumeConfig{Type: composetypes.VolumeTypeBind, Source: "/data", Target: "/t", Bind: &b}
	}
	volume := func(v composetypes.ServiceVolumeVolume, b *composetypes.ServiceVolumeBind) composetypes.ServiceVolumeConfig {
		return composetypes.ServiceVolumeConfig{Type: composetypes.VolumeTypeVolume, Source: "vol", Target: "/t", Volume: &v, Bind: b}
	}
	with := func(v composetypes.ServiceVolumeConfig, change func(*composetypes.ServiceVolumeConfig)) composetypes.ServiceVolumeConfig {
		change(&v)
		return v
	}
	create := composetypes.ServiceVolumeBind{CreateHostPath: true}
	ext := composetypes.Extensions{"x-note": true}

	tests := []struct {
		name   string
		volume composetypes.ServiceVolumeConfig
		mount  bool   // the Mount API is needed
		err    string // the engine can't take the volume
	}{
		// what the bind string says
		{name: "bind", volume: bind(create)},
		{name: "bind without options", volume: composetypes.ServiceVolumeConfig{Type: composetypes.VolumeTypeBind, Source: "/data", Target: "/t"}},
		{name: "bind selinux", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "z"})},
		{name: "bind propagation", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, Propagation: "rshared"})},
		{name: "bind selinux and propagation", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "Z", Propagation: "rslave"})},
		{name: "named volume", volume: volume(composetypes.ServiceVolumeVolume{}, nil)},
		{name: "named volume nocopy", volume: volume(composetypes.ServiceVolumeVolume{NoCopy: true}, nil)},
		{name: "named volume selinux", volume: volume(composetypes.ServiceVolumeVolume{}, &composetypes.ServiceVolumeBind{SELinux: "z"})},

		// what only the Mount API says
		{name: "bind with a consistency", volume: with(bind(create), func(v *composetypes.ServiceVolumeConfig) { v.Consistency = "cached" }), mount: true},
		{name: "bind not creating the host path", volume: bind(composetypes.ServiceVolumeBind{}), mount: true},
		{name: "bind recursive", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, Recursive: "disabled"}), mount: true},
		{name: "volume subpath", volume: volume(composetypes.ServiceVolumeVolume{Subpath: "a"}, nil), mount: true},
		{name: "volume labels", volume: volume(composetypes.ServiceVolumeVolume{Labels: composetypes.Mapping{"a": "b"}}, nil), mount: true},
		{name: "colon in the target", volume: with(bind(create), func(v *composetypes.ServiceVolumeConfig) { v.Target = "/t:x" }), mount: true},
		{name: "tmpfs", volume: composetypes.ServiceVolumeConfig{Type: composetypes.VolumeTypeTmpfs, Target: "/t"}, mount: true},
		{name: "image", volume: composetypes.ServiceVolumeConfig{Type: composetypes.VolumeTypeImage, Source: "alpine", Target: "/t"}, mount: true},

		// the extensions are not for the engine: they change nothing
		{name: "bind with an extension", volume: with(bind(create), func(v *composetypes.ServiceVolumeConfig) { v.Extensions = ext })},
		{name: "bind option with an extension", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, Extensions: ext})},
		{name: "named volume option with an extension", volume: volume(composetypes.ServiceVolumeVolume{NoCopy: true, Extensions: ext}, nil)},
		{name: "bind selinux and an extension", volume: with(bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "z"}), func(v *composetypes.ServiceVolumeConfig) {
			v.Extensions = ext
		})},
		{name: "a volume that needs the Mount API and an extension", volume: with(bind(composetypes.ServiceVolumeBind{}), func(v *composetypes.ServiceVolumeConfig) { v.Extensions = ext }), mount: true},

		// what the engine can't take
		{name: "selinux of a bind that doesn't create its host path", volume: bind(composetypes.ServiceVolumeBind{SELinux: "z"}), err: `selinux "z" can't be applied`},
		{name: "selinux of a recursive bind", volume: bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "Z", Recursive: "readonly"}), err: `selinux "Z" can't be applied`},
		{
			name: "selinux of a bind with a consistency", err: `selinux "z" can't be applied`,
			volume: with(bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "z"}), func(v *composetypes.ServiceVolumeConfig) { v.Consistency = "delegated" }),
		},
		{
			name: "selinux of a volume subpath", err: `selinux "z" can't be applied`,
			volume: volume(composetypes.ServiceVolumeVolume{Subpath: "a"}, &composetypes.ServiceVolumeBind{SELinux: "z"}),
		},
		{
			name: "selinux of a volume with labels", err: `selinux "Z" can't be applied`,
			volume: volume(composetypes.ServiceVolumeVolume{Labels: composetypes.Mapping{"a": "b"}}, &composetypes.ServiceVolumeBind{SELinux: "Z"}),
		},
		{
			name: "selinux of a path with a colon", err: `selinux "z" can't be applied`,
			volume: with(bind(composetypes.ServiceVolumeBind{CreateHostPath: true, SELinux: "z"}), func(v *composetypes.ServiceVolumeConfig) { v.Target = "/t:x" }),
		},
		{
			name: "propagation of a named volume", err: `propagation "rshared" only applies to a bind mount, not to a volume`,
			volume: volume(composetypes.ServiceVolumeVolume{}, &composetypes.ServiceVolumeBind{Propagation: "rshared"}),
		},
		{
			name: "propagation and selinux of a named volume", err: `propagation "rslave" only applies to a bind mount`,
			volume: volume(composetypes.ServiceVolumeVolume{}, &composetypes.ServiceVolumeBind{SELinux: "z", Propagation: "rslave"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.volume
			mount, err := volumeNeedsMountAPI(tt.volume)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, mount, tt.mount)
			// the volume it was given is left as it was
			assert.DeepEqual(t, tt.volume, before)
		})
	}
}

// What reaches the engine for the volumes a service declares, and when it refuses them.
func Test_buildContainerVolumesEngineSupport(t *testing.T) {
	pwd, err := os.Getwd()
	assert.NilError(t, err)
	data := filepath.Join(pwd, "data")

	tests := []struct {
		name   string
		yaml   string
		binds  []string
		mounts []mountTypes.Mount
		err    string
	}{
		{
			name: "selinux stays in the bind string",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        bind:
          selinux: z
`,
			binds: []string{data + ":/data:rw,z"},
		},
		{
			// the Mount API was chosen for the propagation, which dropped the relabeling
			name: "selinux and propagation stay in the bind string",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        bind:
          selinux: Z
          propagation: rshared
`,
			binds: []string{data + ":/data:rw,Z,rshared"},
		},
		{
			name: "selinux of a volume stays in the bind string",
			yaml: `
services:
  test:
    volumes:
      - data:/data:z
volumes:
  data:
    name: my_volume
`,
			binds: []string{"my_volume:/data:rw,z"},
		},
		{
			name: "an extension doesn't send the volume to the Mount API",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        x-note: kept
        bind:
          selinux: z
`,
			binds: []string{data + ":/data:rw,z"},
		},
		{
			name: "a consistency is sent through the Mount API, not dropped from a bind string",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        consistency: cached
`,
			mounts: []mountTypes.Mount{{
				Type: "bind", Source: data, Target: "/data", Consistency: "cached",
			}},
		},
		{
			name: "the engine can't relabel a bind that has a consistency",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        consistency: cached
        bind:
          selinux: z
`,
			err: `service "test": volume /data: selinux "z" can't be applied`,
		},
		{
			name: "a bind that doesn't create its host path goes through the Mount API",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        bind:
          create_host_path: false
`,
			mounts: []mountTypes.Mount{{
				Type: "bind", Source: data, Target: "/data", BindOptions: &mountTypes.BindOptions{CreateMountpoint: false},
			}},
		},
		{
			name: "an extension doesn't hide what needs the Mount API",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        x-note: kept
        bind:
          create_host_path: false
`,
			mounts: []mountTypes.Mount{{
				Type: "bind", Source: data, Target: "/data", BindOptions: &mountTypes.BindOptions{CreateMountpoint: false},
			}},
		},
		{
			name: "the engine can't relabel a bind that doesn't create its host path",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        bind:
          create_host_path: false
          selinux: z
`,
			err: `service "test": volume /data: selinux "z" can't be applied`,
		},
		{
			name: "the engine can't relabel a volume subpath",
			yaml: `
services:
  test:
    volumes:
      - type: volume
        source: data
        target: /data
        volume:
          subpath: sub
        bind:
          selinux: Z
volumes:
  data:
`,
			err: `service "test": volume /data: selinux "Z" can't be applied`,
		},
		{
			name: "the engine can't propagate a volume",
			yaml: `
services:
  test:
    volumes:
      - data:/data:rshared
volumes:
  data:
`,
			err: `service "test": volume /data: propagation "rshared" only applies to a bind mount, not to a volume`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := composeloader.LoadWithContext(t.Context(), composetypes.ConfigDetails{
				ConfigFiles: []composetypes.ConfigFile{{Filename: "test", Content: []byte(tt.yaml)}},
			}, func(options *composeloader.Options) {
				options.SkipValidation = true
				options.SkipConsistencyCheck = true
			})
			assert.NilError(t, err)
			s := &composeService{}
			binds, mounts, err := s.buildContainerVolumes(t.Context(), *p, p.Services["test"], nil)
			if tt.err != "" {
				assert.ErrorContains(t, err, tt.err)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, tt.binds, binds)
			assert.DeepEqual(t, tt.mounts, mounts)
		})
	}
}
