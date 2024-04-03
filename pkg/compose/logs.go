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
	"errors"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Logs(
	ctx context.Context,
	projectName string,
	consumer api.LogConsumer,
	options api.LogOptions,
) error {

	var containers Containers
	var err error

	if options.Index > 0 {
		container, err := s.getSpecifiedContainer(ctx, projectName, oneOffExclude, true, options.Services[0], options.Index)
		if err != nil {
			return err
		}
		containers = append(containers, container)
	} else {
		containers, err = s.getContainers(ctx, projectName, oneOffExclude, true, options.Services...)
		if err != nil {
			return err
		}
	}

	if options.Project != nil && len(options.Services) == 0 {
		// we run with an explicit compose.yaml, so only consider services defined in this file
		options.Services = options.Project.ServiceNames()
		containers = containers.filter(isService(options.Services...))
	}

	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		c := c
		eg.Go(func() error {
			err := s.logContainers(ctx, consumer, c, options)
			var notImplErr errdefs.ErrNotImplemented
			if errors.As(err, &notImplErr) {
				logrus.Warnf("Can't retrieve logs for %q: %s", getCanonicalContainerName(c), err.Error())
				return nil
			}
			return err
		})
	}

	if options.Follow {
		containers = containers.filter(isRunning())
		printer := newLogPrinter(consumer)
		eg.Go(func() error {
			_, err := printer.Run(api.CascadeIgnore, "", nil)
			return err
		})

		for _, c := range containers {
			printer.HandleEvent(api.ContainerEvent{
				Type:      api.ContainerEventAttach,
				Container: getContainerNameWithoutProject(c),
				ID:        c.ID,
				Service:   c.Labels[api.ServiceLabel],
			})
		}

		eg.Go(func() error {
			err := s.watchContainers(ctx, projectName, options.Services, nil, printer.HandleEvent, containers, func(c types.Container, t time.Time) error {
				printer.HandleEvent(api.ContainerEvent{
					Type:      api.ContainerEventAttach,
					Container: getContainerNameWithoutProject(c),
					ID:        c.ID,
					Service:   c.Labels[api.ServiceLabel],
				})
				eg.Go(func() error {
					err := s.logContainers(ctx, consumer, c, api.LogOptions{
						Follow:     options.Follow,
						Since:      t.Format(time.RFC3339Nano),
						Until:      options.Until,
						Tail:       options.Tail,
						Timestamps: options.Timestamps,
					})
					var notImplErr errdefs.ErrNotImplemented
					if errors.As(err, &notImplErr) {
						// ignore
						return nil
					}
					return err
				})
				return nil
			}, func(c types.Container, t time.Time) error {
				printer.HandleEvent(api.ContainerEvent{
					Type:      api.ContainerEventAttach,
					Container: "", // actual name will be set by start event
					ID:        c.ID,
					Service:   c.Labels[api.ServiceLabel],
				})
				return nil
			})
			printer.Stop()
			return err
		})
	}

	return eg.Wait()
}

func (s *composeService) logContainers(ctx context.Context, consumer api.LogConsumer, c types.Container, options api.LogOptions) error {
	cnt, err := s.apiClient().ContainerInspect(ctx, c.ID)
	if err != nil {
		return err
	}

	r, err := s.apiClient().ContainerLogs(ctx, cnt.ID, containerType.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     options.Follow,
		Since:      options.Since,
		Until:      options.Until,
		Tail:       options.Tail,
		Timestamps: options.Timestamps,
	})
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	name := getContainerNameWithoutProject(c)
	w := utils.GetWriter(func(line string) {
		consumer.Log(name, line)
	})
	if cnt.Config.Tty {
		_, err = io.Copy(w, r)
	} else {
		_, err = stdcopy.StdCopy(w, w, r)
	}
	return err
}
