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
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/projectloader"
	"github.com/docker/compose/v5/pkg/api"
)

// Loading a project, or generating one from containers, is done in
// internal/projectloader: composeService hands it the engine it works with.

func (s *composeService) projectLoader() *projectloader.Service {
	return projectloader.New(projectLoaderBackend{s})
}

// LoadProject implements api.Compose.LoadProject
// It loads and validates a Compose project from configuration files.
func (s *composeService) LoadProject(ctx context.Context, options api.ProjectLoadOptions) (*types.Project, error) {
	return s.projectLoader().LoadProject(ctx, options)
}

// Generate implements api.Compose.Generate
// It generates a Compose project from existing containers.
func (s *composeService) Generate(ctx context.Context, options api.GenerateOptions) (*types.Project, error) {
	return s.projectLoader().Generate(ctx, options)
}

// projectLoaderBackend is the engine of a composeService, as internal/projectloader sees it.
type projectLoaderBackend struct {
	s *composeService
}

func (b projectLoaderBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b projectLoaderBackend) CLI() command.Cli {
	return b.s.dockerCli
}
