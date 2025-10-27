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
	"fmt"
	"strings"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/docker/api/types/container"
)

func (s *composeService) Commit(ctx context.Context, projectName string, options api.CommitOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.commit(ctx, projectName, options)
	}, "commit", s.events)
}

func (s *composeService) commit(ctx context.Context, projectName string, options api.CommitOptions) error {
	projectName = strings.ToLower(projectName)

	ctr, err := s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, options.Service, options.Index)
	if err != nil {
		return err
	}

	name := getCanonicalContainerName(ctr)
	msg := fmt.Sprintf("Commit %s", name)

	s.events.On(progress.Event{
		ID:         name,
		Text:       msg,
		Status:     progress.Working,
		StatusText: "Committing",
	})

	if s.dryRun {
		s.events.On(progress.Event{
			ID:         name,
			Text:       msg,
			Status:     progress.Done,
			StatusText: "Committed",
		})

		return nil
	}

	response, err := s.apiClient().ContainerCommit(ctx, ctr.ID, container.CommitOptions{
		Reference: options.Reference,
		Comment:   options.Comment,
		Author:    options.Author,
		Changes:   options.Changes.GetSlice(),
		Pause:     options.Pause,
	})
	if err != nil {
		return err
	}

	s.events.On(progress.Event{
		ID:         name,
		Text:       msg,
		Status:     progress.Done,
		StatusText: fmt.Sprintf("Committed as %s", response.ID),
	})

	return nil
}
