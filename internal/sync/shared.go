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

package sync

import (
	"context"

	"github.com/compose-spec/compose-go/v2/types"
)

// PathMapping contains the Compose service and modified host system path.
type PathMapping struct {
	// HostPath that was created/modified/deleted outside the container.
	//
	// This is the path as seen from the user's perspective, e.g.
	// 	- C:\Users\moby\Documents\hello-world\main.go (file on Windows)
	//  - /Users/moby/Documents/hello-world (directory on macOS)
	HostPath string
	// ContainerPath for the target file inside the container (only populated
	// for sync events, not rebuild).
	//
	// This is the path as used in Docker CLI commands, e.g.
	//	- /workdir/main.go
	//  - /workdir/subdir
	ContainerPath string
}

type Syncer interface {
	Sync(ctx context.Context, service types.ServiceConfig, paths []PathMapping) error
}
