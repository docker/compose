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

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/engine"
)

// engineBackend is the engine of a composeService, as the packages extracted out of
// pkg/compose (internal/inspect, internal/containerops) see it.
type engineBackend struct {
	s *composeService
}

func (b engineBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b engineBackend) MaxConcurrency() int {
	return b.s.maxConcurrency
}

func (b engineBackend) Containers(ctx context.Context, projectName string, oneOff engine.OneOff, all bool, services ...string) ([]container.Summary, error) {
	return b.s.getContainers(ctx, projectName, toOneOff(oneOff), all, services...)
}

func (b engineBackend) SpecifiedContainer(ctx context.Context, projectName string, oneOff engine.OneOff, all bool, serviceName string, index int) (container.Summary, error) {
	return b.s.getSpecifiedContainer(ctx, projectName, toOneOff(oneOff), all, serviceName, index)
}

func toOneOff(v engine.OneOff) oneOff {
	if v == engine.OneOffExclude {
		return oneOffExclude
	}
	return oneOffInclude
}
