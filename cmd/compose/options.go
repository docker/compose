/*
   Copyright 2023 Docker Compose CLI authors

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
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/utils"
)

func applyPlatforms(project *types.Project, buildForSinglePlatform bool) error {
	defaultPlatform := project.Environment["DOCKER_DEFAULT_PLATFORM"]
	for name, service := range project.Services {
		if service.Build == nil {
			continue
		}

		// default platform only applies if the service doesn't specify
		if defaultPlatform != "" && service.Platform == "" {
			if len(service.Build.Platforms) > 0 && !utils.StringContains(service.Build.Platforms, defaultPlatform) {
				return fmt.Errorf("service %q build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: %s", name, defaultPlatform)
			}
			service.Platform = defaultPlatform
		}

		if service.Platform != "" {
			if len(service.Build.Platforms) > 0 {
				if !utils.StringContains(service.Build.Platforms, service.Platform) {
					return fmt.Errorf("service %q build configuration does not support platform: %s", name, service.Platform)
				}
			}

			if buildForSinglePlatform || len(service.Build.Platforms) == 0 {
				// if we're building for a single platform, we want to build for the platform we'll use to run the image
				// similarly, if no build platforms were explicitly specified, it makes sense to build for the platform
				// the image is designed for rather than allowing the builder to infer the platform
				service.Build.Platforms = []string{service.Platform}
			}
		}

		// services can specify that they should be built for multiple platforms, which can be used
		// with `docker compose build` to produce a multi-arch image
		// other cases, such as `up` and `run`, need a single architecture to actually run
		// if there is only a single platform present (which might have been inferred
		// from service.Platform above), it will be used, even if it requires emulation.
		// if there's more than one platform, then the list is cleared so that the builder
		// can decide.
		// TODO(milas): there's no validation that the platform the builder will pick is actually one
		// 	of the supported platforms from the build definition
		// 	e.g. `build.platforms: [linux/arm64, linux/amd64]` on a `linux/ppc64` machine would build
		// 	for `linux/ppc64` instead of returning an error that it's not a valid platform for the service.
		if buildForSinglePlatform && len(service.Build.Platforms) > 1 {
			// empty indicates that the builder gets to decide
			service.Build.Platforms = nil
		}
		project.Services[name] = service
	}
	return nil
}
