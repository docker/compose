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

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Deploy(ctx context.Context, project *types.Project, options api.DeployOptions) error {
	if options.Build != nil {
		if err := s.Build(ctx, project, *options.Build); err != nil {
			return err
		}
	}

	if options.Push {
		if err := s.Push(ctx, project, api.PushOptions{
			Quiet: options.Quiet,
		}); err != nil {
			return err
		}
	}

	return s.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{
			Services:             options.Services,
			Recreate:             api.RecreateForce,
			RecreateDependencies: api.RecreateForce,
			RemoveOrphans:        options.RemoveOrphans,
			Inherit:              true,
			QuietPull:            options.Quiet,
		},
		Start: api.StartOptions{
			Project:     project,
			Services:    options.Services,
			Wait:        options.Wait,
			WaitTimeout: options.WaitTimeout,
		},
	})
}
