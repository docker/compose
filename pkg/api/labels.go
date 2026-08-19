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

package api

import (
	"github.com/hashicorp/go-version"

	"github.com/docker/compose/v5/internal"
)

const (
	// ProjectLabel allow to track resource related to a compose project
	ProjectLabel = "com.docker.compose.project"
	// ServiceLabel allow to track resource related to a compose service
	ServiceLabel = "com.docker.compose.service"
	// ConfigHashLabel stores configuration hash for a compose service
	ConfigHashLabel = "com.docker.compose.config-hash"
	// ContainerNumberLabel stores the container index of a replicated service
	ContainerNumberLabel = "com.docker.compose.container-number"
	// VolumeLabel allow to track resource related to a compose volume
	VolumeLabel = "com.docker.compose.volume"
	// NetworkLabel allow to track resource related to a compose network
	NetworkLabel = "com.docker.compose.network"
	// WorkingDirLabel stores absolute path to compose project working directory
	WorkingDirLabel = "com.docker.compose.project.working_dir"
	// ConfigFilesLabel stores absolute path to compose project configuration files
	ConfigFilesLabel = "com.docker.compose.project.config_files"
	// EnvironmentFileLabel stores absolute path to compose project env file set by `--env-file`
	EnvironmentFileLabel = "com.docker.compose.project.environment_file"
	// OneoffLabel stores value 'True' for one-off containers created by `compose run`
	OneoffLabel = "com.docker.compose.oneoff"
	// SlugLabel stores unique slug used for one-off container identity
	SlugLabel = "com.docker.compose.slug"
	// ImageDigestLabel stores digest of the container image used to run service
	ImageDigestLabel = "com.docker.compose.image"
	// ImageVolumeDigestLabel stores the content digest of each `type: image`
	// volume's source image, as "target=digest" pairs joined by ",", so
	// mustRecreate can detect a rebuilt/updated source image independently of
	// the mount Source (which must stay a resolvable name, not a digest).
	ImageVolumeDigestLabel = "com.docker.compose.image-volume-digest"
	// DependenciesLabel stores service dependencies
	DependenciesLabel = "com.docker.compose.depends_on"
	// VersionLabel stores the compose tool version used to build/run application
	VersionLabel = "com.docker.compose.version"
	// ImageBuilderLabel stores the builder (classic or BuildKit) used to produce the image.
	ImageBuilderLabel = "com.docker.compose.image.builder"
	// ContainerReplaceLabel is set when container is created to replace another container (recreated)
	ContainerReplaceLabel = "com.docker.compose.replace"
	// ContainerEngineLabel stores the name of the engine that runs the container
	ContainerEngineLabel = "com.docker.compose.engine"
	// HookLabel identifies an ephemeral lifecycle-hook container and stores the
	// hook type, e.g. "pre_start". It is applied to every container created by
	// runPreStartHook so orphan hook containers from a previous failed run can
	// be found and removed by project+service+hook label filters.
	HookLabel = "com.docker.compose.hook"
)

// ComposeVersion is the compose tool version as declared by label VersionLabel
var ComposeVersion string

func init() {
	v, err := version.NewVersion(internal.Version)
	if err == nil {
		ComposeVersion = v.Core().String()
	}
}
