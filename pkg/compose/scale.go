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
	"maps"
	"slices"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Scale(ctx context.Context, project *types.Project, options api.ScaleOptions) error {
	return Run(ctx, tracing.SpanWrapFunc("project/scale", tracing.ProjectOptions(ctx, project), func(ctx context.Context) error {
		if err := applyReplicas(project, options.Replicas); err != nil {
			return err
		}
		services := options.Services
		if len(services) == 0 {
			services = slices.Collect(maps.Keys(options.Replicas))
		}
		err := s.create(ctx, project, api.CreateOptions{Services: services})
		if err != nil {
			return err
		}
		return s.start(ctx, project.Name, api.StartOptions{Project: project, Services: services}, nil)
	}), "scale", s.events)
}

// applyReplicas applies the requested replica counts to the project model,
// which is what the convergence performed by create/start acts upon.
func applyReplicas(project *types.Project, replicas map[string]int) error {
	for name, scale := range replicas {
		service, err := project.GetService(name)
		if err != nil {
			return err
		}
		service.SetScale(scale)
		project.Services[name] = service
	}
	return nil
}
