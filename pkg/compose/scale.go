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
	"github.com/docker/compose/v2/internal/tracing"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Scale(ctx context.Context, project *types.Project, options api.ScaleOptions) error {
	return progress.Run(ctx, tracing.SpanWrapFunc("project/scale", tracing.ProjectOptions(ctx, project), func(ctx context.Context) error {
		err := s.create(ctx, project, api.CreateOptions{Services: options.Services})
		if err != nil {
			return err
		}
		return s.start(ctx, project.Name, api.StartOptions{Project: project, Services: options.Services}, nil)

	}), s.stdinfo())
}
