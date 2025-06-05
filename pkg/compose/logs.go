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
	"io"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
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
		ctr, err := s.getSpecifiedContainer(ctx, projectName, oneOffExclude, true, options.Services[0], options.Index)
		if err != nil {
			return err
		}
		containers = append(containers, ctr)
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
	for _, ctr := range containers {
		eg.Go(func() error {
			err := s.logContainer(ctx, consumer, ctr, options)
			if errdefs.IsNotImplemented(err) {
				logrus.Warnf("Can't retrieve logs for %q: %s", getCanonicalContainerName(ctr), err.Error())
				return nil
			}
			return err
		})
	}

	if options.Follow {
		printer := newLogPrinter(consumer)
		eg.Go(printer.Run)

		monitor := newMonitor(s.apiClient(), options.Project)
		monitor.withListener(func(event api.ContainerEvent) {
			if event.Type == api.ContainerEventStarted {
				eg.Go(func() error {
					ctr, err := s.apiClient().ContainerInspect(ctx, event.ID)
					if err != nil {
						return err
					}

					err = s.doLogContainer(ctx, consumer, event.Source, ctr, api.LogOptions{
						Follow:     options.Follow,
						Since:      time.Unix(0, event.Time).Format(time.RFC3339Nano),
						Until:      options.Until,
						Tail:       options.Tail,
						Timestamps: options.Timestamps,
					})
					if errdefs.IsNotImplemented(err) {
						// ignore
						return nil
					}
					return err
				})
			}
		})
		eg.Go(func() error {
			defer printer.Stop()
			return monitor.Start(ctx)
		})
	}

	return eg.Wait()
}

func (s *composeService) logContainer(ctx context.Context, consumer api.LogConsumer, c container.Summary, options api.LogOptions) error {
	ctr, err := s.apiClient().ContainerInspect(ctx, c.ID)
	if err != nil {
		return err
	}
	name := getContainerNameWithoutProject(c)
	return s.doLogContainer(ctx, consumer, name, ctr, options)
}

func (s *composeService) doLogContainer(ctx context.Context, consumer api.LogConsumer, name string, ctr container.InspectResponse, options api.LogOptions) error {
	r, err := s.apiClient().ContainerLogs(ctx, ctr.ID, container.LogsOptions{
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

	w := utils.GetWriter(func(line string) {
		consumer.Log(name, line)
	})
	if ctr.Config.Tty {
		_, err = io.Copy(w, r)
	} else {
		_, err = stdcopy.StdCopy(w, w, r)
	}
	return err
}
