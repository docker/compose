// +build kube

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

package resources

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
)

const dockerSock = "/var/run/docker.sock"

type volumeSpec struct {
	mount  apiv1.VolumeMount
	source *apiv1.VolumeSource
}

func toVolumeSpecs(project *types.Project, s types.ServiceConfig) ([]volumeSpec, error) {
	var specs []volumeSpec
	for i, m := range s.Volumes {
		var source *apiv1.VolumeSource
		name := fmt.Sprintf("mount-%d", i)
		subpath := ""
		if m.Source == dockerSock && m.Target == dockerSock {
			subpath = "docker.sock"
			source = hostPathVolume("/var/run")
		} else if strings.HasSuffix(m.Source, ".git") {
			source = gitVolume(m.Source)
		} else if m.Type == "volume" {
			if m.Source != "" {
				name = strings.ReplaceAll(m.Source, "_", "-")
			}
		} else {
			// bind mount
			if !filepath.IsAbs(m.Source) {
				return nil, errors.Errorf("%s: only absolute paths can be specified in mount source", m.Source)
			}
			if m.Source == "/" {
				source = hostPathVolume("/")
			} else {
				parent, file := filepath.Split(m.Source)
				if parent != "/" {
					parent = strings.TrimSuffix(parent, "/")
				}
				source = hostPathVolume(parent)
				subpath = file
			}
		}

		specs = append(specs, volumeSpec{
			source: source,
			mount:  volumeMount(name, m.Target, m.ReadOnly, subpath),
		})
	}

	for i, m := range s.Tmpfs {
		name := fmt.Sprintf("tmp-%d", i)

		specs = append(specs, volumeSpec{
			source: emptyVolumeInMemory(),
			mount:  volumeMount(name, m, false, ""),
		})
	}

	for _, s := range s.Secrets {
		name := fmt.Sprintf("%s-%s", project.Name, s.Source)
		target := path.Join("/run/secrets", or(s.Target, path.Join(s.Source, s.Source)))

		specs = append(specs, secretMount(name, target))
	}

	for i, c := range s.Configs {
		name := fmt.Sprintf("config-%d", i)

		target := or(c.Target, "/"+c.Source)
		subPath := name
		readOnly := true

		specs = append(specs, volumeSpec{
			source: configVolume(c, project.Configs[name], subPath),
			mount:  volumeMount(name, target, readOnly, subPath),
		})
	}

	return specs, nil
}

func or(v string, defaultValue string) string {
	if v != "" && v != "." {
		return v
	}

	return defaultValue
}

func toVolumeMounts(project *types.Project, s types.ServiceConfig) ([]apiv1.VolumeMount, error) {
	var mounts []apiv1.VolumeMount
	specs, err := toVolumeSpecs(project, s)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		mounts = append(mounts, spec.mount)
	}
	return mounts, nil
}

func toVolumes(project *types.Project, s types.ServiceConfig) ([]apiv1.Volume, error) {
	var volumes []apiv1.Volume
	specs, err := toVolumeSpecs(project, s)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if spec.source == nil {
			spec.source = emptyVolumeInMemory()
		}
		volumes = append(volumes, apiv1.Volume{
			Name:         spec.mount.Name,
			VolumeSource: *spec.source,
		})
	}
	return volumes, nil
}

func gitVolume(path string) *apiv1.VolumeSource {
	return &apiv1.VolumeSource{
		GitRepo: &apiv1.GitRepoVolumeSource{
			Repository: filepath.ToSlash(path),
		},
	}
}

func hostPathVolume(path string) *apiv1.VolumeSource {
	return &apiv1.VolumeSource{
		HostPath: &apiv1.HostPathVolumeSource{
			Path: path,
		},
	}
}

func defaultMode(mode *uint32) *int32 {
	var defaultMode *int32

	if mode != nil {
		signedMode := int32(*mode)
		defaultMode = &signedMode
	}

	return defaultMode
}

func secretMount(name, target string) volumeSpec {
	readOnly := true

	filename := filepath.Base(target)
	dir := filepath.Dir(target)

	return volumeSpec{
		source: &apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: name,
				Items: []apiv1.KeyToPath{
					{
						Key:  name,
						Path: filename,
					},
				},
			},
		},
		mount: apiv1.VolumeMount{
			Name:      filename,
			MountPath: dir,
			ReadOnly:  readOnly,
		},
	}
}

func volumeMount(name, path string, readOnly bool, subPath string) apiv1.VolumeMount {
	return apiv1.VolumeMount{
		Name:      name,
		MountPath: path,
		ReadOnly:  readOnly,
		SubPath:   subPath,
	}
}

func configVolume(config types.ServiceConfigObjConfig, topLevelConfig types.ConfigObjConfig, subPath string) *apiv1.VolumeSource {
	return &apiv1.VolumeSource{
		ConfigMap: &apiv1.ConfigMapVolumeSource{
			LocalObjectReference: apiv1.LocalObjectReference{
				Name: config.Source,
			},
			Items: []apiv1.KeyToPath{
				{
					Key:  toKey(topLevelConfig.File),
					Path: subPath,
					Mode: defaultMode(config.Mode),
				},
			},
		},
	}
}

func toKey(file string) string {
	if file != "" {
		return path.Base(file)
	}

	return "file" // TODO: hard-coded key for external configs
}

func emptyVolumeInMemory() *apiv1.VolumeSource {
	return &apiv1.VolumeSource{
		EmptyDir: &apiv1.EmptyDirVolumeSource{
			Medium: apiv1.StorageMediumMemory,
		},
	}
}
