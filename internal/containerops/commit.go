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

package containerops

import (
	"context"
	"strings"

	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
)

// Commit creates an image from the changes made to a container of a service.
func (s *Service) Commit(ctx context.Context, projectName string, options api.CommitOptions) error {
	projectName = strings.ToLower(projectName)

	ctr, err := s.backend.SpecifiedContainer(ctx, projectName, engine.OneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	name := naming.CanonicalContainerName(ctr)

	s.backend.Events().On(api.Resource{
		ID:     name,
		Status: api.Working,
		Text:   api.StatusCommitting,
	})

	if s.backend.DryRun() {
		s.backend.Events().On(api.Resource{
			ID:     name,
			Status: api.Done,
			Text:   api.StatusCommitted,
		})

		return nil
	}

	response, err := s.backend.APIClient().ContainerCommit(ctx, ctr.ID, client.ContainerCommitOptions{
		Reference: options.Reference,
		Comment:   options.Comment,
		Author:    options.Author,
		Changes:   options.Changes.GetSlice(),
		NoPause:   !options.Pause,
	})
	if err != nil {
		return err
	}

	s.backend.Events().On(api.Resource{
		ID:     name,
		Text:   "Committed as " + response.ID,
		Status: api.Done,
	})

	return nil
}
