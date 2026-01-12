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

// Docker Engine API version constants.
// These versions correspond to specific Docker Engine releases and their features.
const (
	// APIVersion144 represents Docker Engine API version 1.44 (Engine v25.0).
	//
	// New features in this version:
	//  - Endpoint-specific MAC address configuration
	//  - Multiple networks can be connected during container creation
	//  - healthcheck.start_interval parameter support
	//
	// Before this version:
	//  - MAC address was container-wide only
	//  - Extra networks required post-creation NetworkConnect calls
	//  - healthcheck.start_interval was not available
	APIVersion144 = "1.44"

	// APIVersion148 represents Docker Engine API version 1.48 (Engine v28.0).
	//
	// New features in this version:
	//  - Volume mounts with type=image support
	//
	// Before this version:
	//  - Only bind, volume, and tmpfs mount types were supported
	APIVersion148 = "1.48"

	// APIVersion149 represents Docker Engine API version 1.49 (Engine v28.1).
	//
	// New features in this version:
	//  - Network interface_name configuration
	//  - Platform parameter in ImageList API
	//
	// Before this version:
	//  - interface_name was not configurable
	//  - ImageList didn't support platform filtering
	APIVersion149 = "1.49"
)

// Docker Engine version strings for user-facing error messages.
// These should be used in error messages to provide clear version requirements.
const (
	// DockerEngineV25 is the major version string for Docker Engine 25.x
	DockerEngineV25 = "v25"

	// DockerEngineV28 is the major version string for Docker Engine 28.x
	DockerEngineV28 = "v28"

	// DockerEngineV28_1 is the specific version string for Docker Engine 28.1
	DockerEngineV28_1 = "v28.1"
)

// Build tool version constants
const (
	// BuildxMinVersion is the minimum required version of buildx for compose build
	BuildxMinVersion = "0.17.0"
)
