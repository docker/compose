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

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"
	status "github.com/docker/compose-cli/local/moby"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Remove(ctx context.Context, project *types.Project, options compose.RemoveOptions) ([]string, error) {
	containers, err := s.getContainers(ctx, project, oneOffInclude, nil)
	if err != nil {
		return nil, err
	}

	stoppedContainers := containers.filter(func(c moby.Container) bool {
		return c.State != status.ContainerRunning
	})

	var names []string
	stoppedContainers.forEach(func(c moby.Container) {
		names = append(names, getCanonicalContainerName(c))
	})

	if options.DryRun {
		return names, nil
	}

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range stoppedContainers {
		c := c
		eg.Go(func() error {
			eventName := getContainerProgressName(c)
			w.Event(progress.RemovingEvent(eventName))
			err = s.apiClient.ContainerRemove(ctx, c.ID, moby.ContainerRemoveOptions{
				RemoveVolumes: options.Volumes,
				Force:         options.Force,
			})
			if err == nil {
				w.Event(progress.RemovedEvent(eventName))
			}
			return err
		})
	}
	return nil, eg.Wait()
}
