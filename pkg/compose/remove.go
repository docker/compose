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
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/prompt"
)

func (s *composeService) Remove(ctx context.Context, projectName string, options api.RemoveOptions) error {
	projectName = strings.ToLower(projectName)

	if options.Stop {
		err := s.Stop(ctx, projectName, api.StopOptions{
			Services: options.Services,
			Project:  options.Project,
		})
		if err != nil {
			return err
		}
	}

	containers, err := s.getContainers(ctx, projectName, oneOffExclude, true, options.Services...)
	if err != nil {
		if api.IsNotFoundError(err) {
			_, _ = fmt.Fprintln(s.stderr(), "No stopped containers")
			return nil
		}
		return err
	}

	if options.Project != nil {
		containers = containers.filter(isService(options.Project.ServiceNames()...))
	}

	var stoppedContainers Containers
	for _, container := range containers {
		// We have to inspect containers, as State reported by getContainers suffers a race condition
		inspected, err := s.apiClient().ContainerInspect(ctx, container.ID)
		if api.IsNotFoundError(err) {
			// Already removed. Maybe configured with auto-remove
			continue
		}
		if err != nil {
			return err
		}
		if !inspected.State.Running || (options.Stop && s.dryRun) {
			stoppedContainers = append(stoppedContainers, container)
		}
	}

	var names []string
	stoppedContainers.forEach(func(c moby.Container) {
		names = append(names, getCanonicalContainerName(c))
	})

	if len(names) == 0 {
		_, _ = fmt.Fprintln(s.stdinfo(), "No stopped containers")
		return nil
	}
	msg := fmt.Sprintf("Going to remove %s", strings.Join(names, ", "))
	if options.Force {
		_, _ = fmt.Fprintln(s.stdout(), msg)
	} else {
		confirm, err := prompt.NewPrompt(s.stdin(), s.stdout()).Confirm(msg, false)
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}
	return progress.RunWithTitle(ctx, func(ctx context.Context) error {
		return s.remove(ctx, stoppedContainers, options)
	}, s.stdinfo(), "Removing")
}

func (s *composeService) remove(ctx context.Context, containers Containers, options api.RemoveOptions) error {
	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			eventName := getContainerProgressName(container)
			w.Event(progress.RemovingEvent(eventName))
			err := s.apiClient().ContainerRemove(ctx, container.ID, containerType.RemoveOptions{
				RemoveVolumes: options.Volumes,
				Force:         options.Force,
			})
			if err == nil {
				w.Event(progress.RemovedEvent(eventName))
			}
			return err
		})
	}
	return eg.Wait()
}
