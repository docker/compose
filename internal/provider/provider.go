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

// Package provider runs the provider plugins that manage the services declared
// with a provider, and speaks the JSON message protocol they use to publish
// variables and endpoints, and to ask Compose for the service configuration,
// the relay information or an image.
package provider

import (
	"context"
	"os/exec"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/internal/relay"
)

// Backend is what running a provider plugin needs from the engine.
type Backend interface {
	// The Docker client, the event bus and the Docker Desktop detection, which
	// serve the relay deployed for providers that publish endpoints.
	relay.Backend
	// CLI is the Docker CLI, which finds the plugin binary.
	CLI() command.Cli
	// PrepareShellOut sets the environment of a command Compose runs, from the project's.
	PrepareShellOut(ctx context.Context, env types.Mapping, cmd *exec.Cmd) error
	// DryRun tells that the Docker API client is a dry-run one, which cannot intercept the provider processes.
	DryRun() bool
	// ProjectLock guards the project model, which every concurrent provider run mutates.
	ProjectLock() sync.Locker
}

// Service runs provider plugins on a Backend.
type Service struct {
	backend Backend
	relay   *relay.Service
}

// New creates a Service working with the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend, relay: relay.New(backend)}
}
