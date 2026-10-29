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

// Package engine describes what the operations extracted out of pkg/compose
// need from the engine they run against, so that they all state it the same way.
package engine

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// OneOff tells whether the containers of `compose run` are part of a listing.
type OneOff int

const (
	// OneOffInclude lists the one-off containers along with the services' ones.
	OneOffInclude OneOff = iota
	// OneOffExclude leaves the one-off containers out.
	OneOffExclude
)

// Backend is what every operation needs from the engine: the Docker daemon, the
// bound on the calls made to it, and the containers of a project.
type Backend interface {
	// APIClient talks to the Docker daemon.
	APIClient() client.APIClient
	// MaxConcurrency bounds the daemon calls made in parallel, 0 meaning no bound.
	MaxConcurrency() int
	// Containers lists the containers of a project, narrowed to services when given.
	Containers(ctx context.Context, projectName string, oneOff OneOff, all bool, services ...string) ([]container.Summary, error)
	// SpecifiedContainer returns the container of a service, the replica with
	// the given index when it is above zero.
	SpecifiedContainer(ctx context.Context, projectName string, oneOff OneOff, all bool, serviceName string, index int) (container.Summary, error)
}
