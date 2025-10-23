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

// ContextInfo provides Docker context information for advanced scenarios
type ContextInfo interface {
	// CurrentContext returns the name of the current Docker context
	// Returns "default" for simple clients without context support
	CurrentContext() string

	// ServerOSType returns the Docker daemon's operating system (linux/windows/darwin)
	// Used for OS-specific compatibility checks
	ServerOSType() string

	// BuildKitEnabled determines whether BuildKit should be used for builds
	// Checks DOCKER_BUILDKIT env var, config, and daemon capabilities
	BuildKitEnabled() (bool, error)
}
