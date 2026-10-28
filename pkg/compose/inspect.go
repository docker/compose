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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/inspect"
	"github.com/docker/compose/v5/pkg/api"
)

// The read-only operations live in internal/inspect: composeService serves
// them by handing that package the engine it reports on.

func (s *composeService) inspector() *inspect.Service {
	return inspect.New(inspectBackend{s})
}

func (s *composeService) List(ctx context.Context, options api.ListOptions) ([]api.Stack, error) {
	return s.inspector().List(ctx, options)
}

func (s *composeService) Ps(ctx context.Context, projectName string, options api.PsOptions) ([]api.ContainerSummary, error) {
	return s.inspector().Ps(ctx, projectName, options)
}

func (s *composeService) Top(ctx context.Context, projectName string, services []string) ([]api.ContainerProcSummary, error) {
	return s.inspector().Top(ctx, projectName, services)
}

func (s *composeService) Ports(ctx context.Context, projectName string, serviceName string, port uint16, options api.PortOptions) (api.PortPublishers, error) {
	return s.inspector().Ports(ctx, projectName, serviceName, port, options)
}

func (s *composeService) Volumes(ctx context.Context, projectName string, options api.VolumesOptions) ([]api.VolumesSummary, error) {
	return s.inspector().Volumes(ctx, projectName, options)
}

func (s *composeService) Events(ctx context.Context, projectName string, options api.EventsOptions) error {
	if err := options.Validate(); err != nil {
		return err
	}
	return s.inspector().Events(ctx, projectName, options)
}

func (s *composeService) Viz(ctx context.Context, project *types.Project, options api.VizOptions) (string, error) {
	return s.inspector().Viz(ctx, project, options)
}

// inspectBackend is the engine of a composeService, as internal/inspect sees it.
type inspectBackend struct {
	s *composeService
}

func (b inspectBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b inspectBackend) MaxConcurrency() int {
	return b.s.maxConcurrency
}

func (b inspectBackend) Containers(ctx context.Context, projectName string, oneOff inspect.OneOff, all bool, services ...string) ([]container.Summary, error) {
	return b.s.getContainers(ctx, projectName, toOneOff(oneOff), all, services...)
}

func (b inspectBackend) SpecifiedContainer(ctx context.Context, projectName string, oneOff inspect.OneOff, all bool, serviceName string, index int) (container.Summary, error) {
	return b.s.getSpecifiedContainer(ctx, projectName, toOneOff(oneOff), all, serviceName, index)
}

func toOneOff(v inspect.OneOff) oneOff {
	if v == inspect.OneOffExclude {
		return oneOffExclude
	}
	return oneOffInclude
}
