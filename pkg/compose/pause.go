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

	moby "github.com/docker/docker/api/types"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Pause(ctx context.Context, project string, options api.PauseOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.pause(ctx, project, options)
	})
}

func (s *composeService) pause(ctx context.Context, project string, options api.PauseOptions) error {
	containers, err := s.getContainers(ctx, project, oneOffExclude, true, options.Services...)
	if err != nil {
		return err
	}

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	containers.forEach(func(container moby.Container) {
		eg.Go(func() error {
			err := s.apiClient.ContainerPause(ctx, container.ID)
			if err == nil {
				eventName := getContainerProgressName(container)
				w.Event(progress.NewEvent(eventName, progress.Done, "Paused"))
			}
			return err
		})

	})
	return eg.Wait()
}

func (s *composeService) UnPause(ctx context.Context, project string, options api.PauseOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.unPause(ctx, project, options)
	})
}

func (s *composeService) unPause(ctx context.Context, project string, options api.PauseOptions) error {
	containers, err := s.getContainers(ctx, project, oneOffExclude, true, options.Services...)
	if err != nil {
		return err
	}

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	containers.forEach(func(container moby.Container) {
		eg.Go(func() error {
			err = s.apiClient.ContainerUnpause(ctx, container.ID)
			if err == nil {
				eventName := getContainerProgressName(container)
				w.Event(progress.NewEvent(eventName, progress.Done, "Unpaused"))
			}
			return err
		})

	})
	return eg.Wait()
}
