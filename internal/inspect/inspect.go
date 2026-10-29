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

// Package inspect implements the read-only operations that report on a
// project and its resources: ps, ls, top, port, volumes, events and viz.
package inspect

import (
	"slices"

	"github.com/moby/moby/api/types/container"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/pkg/api"
)

// Backend is what the operations need from the engine they report on.
type Backend interface {
	engine.Backend
}

// Service runs the inspection operations on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service reporting on the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}

// inServices keeps the containers of the given services.
func inServices(containers []container.Summary, services []string) []container.Summary {
	var filtered []container.Summary
	for _, ctr := range containers {
		if slices.Contains(services, ctr.Labels[api.ServiceLabel]) {
			filtered = append(filtered, ctr)
		}
	}
	return filtered
}
