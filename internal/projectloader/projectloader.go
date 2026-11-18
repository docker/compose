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

// Package projectloader obtains a Compose project: either by loading and
// validating the Compose files (LoadProject), or by generating one from
// existing containers (Generate).
package projectloader

import (
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client"
)

// Backend is what loading or generating a project needs from the engine.
type Backend interface {
	// APIClient talks to the Docker daemon, to read the containers to generate a project from.
	APIClient() client.APIClient
	// CLI is the Docker CLI, which resolves the remote (Git, OCI) Compose files.
	CLI() command.Cli
}

// Service loads and generates projects on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service working with the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}
