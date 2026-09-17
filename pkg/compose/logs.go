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

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

func (s *composeService) Logs(
	ctx context.Context,
	projectName string,
	consumer api.LogConsumer,
	options api.LogOptions,
) error {
	containers, err := s.selectLogsContainers(ctx, projectName, &options)
	if err != nil {
		return err
	}

	var eg *errgroup.Group
	// limiter bounds how many containers are connecting (ContainerInspect +
	// opening ContainerLogs) at once, in follow mode only: the streams
	// themselves run indefinitely once opened, so gating the whole call
	// (like the non-follow case does) would pin every slot forever and
	// starve the monitor and any later container, the same reason
	// waitDependencies is excluded from the concurrency cap.
	var limiter *semaphore.Weighted
	if options.Follow {
		eg, ctx = errgroup.WithContext(ctx)
		limiter = newOptionalLimiter(s.maxConcurrency)
	} else {
		eg, ctx = newLimitedErrgroup(ctx, s.maxConcurrency)
	}
	for _, ctr := range containers {
		eg.Go(func() error {
			return s.logContainer(ctx, limiter, consumer, ctr, options)
		})
	}

	if options.Follow {
		printer := newLogPrinter(consumer)

		monitor := newMonitor(s.apiClient(), projectName)
		if len(options.Services) > 0 {
			monitor.withServices(options.Services)
		} else if options.Project != nil {
			monitor.withServices(options.Project.ServiceNames())
		}
		monitor.withListener(printer.HandleEvent)
		monitor.withListener(s.followStartedContainersLogs(ctx, eg, limiter, consumer, options))
		eg.Go(func() error {
			// pass ctx so monitor will immediately stop on SIGINT
			return monitor.Start(ctx)
		})
	}

	return eg.Wait()
}

// selectLogsContainers returns the containers to stream logs from, per the
// requested services, container index, and project
func (s *composeService) selectLogsContainers(ctx context.Context, projectName string, options *api.LogOptions) (Containers, error) {
	if options.Index > 0 {
		ctr, err := s.getSpecifiedContainer(ctx, projectName, oneOffExclude, true, options.Services[0], options.Index)
		if err != nil {
			return nil, err
		}
		return Containers{ctr}, nil
	}
	containers, err := s.getContainers(ctx, projectName, oneOffExclude, true, options.Services...)
	if err != nil {
		return nil, err
	}
	if options.Project != nil && len(options.Services) == 0 {
		// we run with an explicit compose.yaml, so only consider services defined in this file
		options.Services = options.Project.ServiceNames()
		containers = containers.filter(isService(options.Services...))
	}
	return containers, nil
}

// logContainer streams a container's logs, warning when its logging driver
// doesn't support reading logs
func (s *composeService) logContainer(ctx context.Context, limiter *semaphore.Weighted, consumer api.LogConsumer, ctr container.Summary, options api.LogOptions) error {
	if err := acquireSlot(ctx, limiter); err != nil {
		return err
	}
	res, err := s.apiClient().ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
	if err != nil {
		releaseSlot(limiter)
		return err
	}
	err = s.doLogContainer(ctx, limiter, consumer, getContainerNameWithoutProject(ctr), res.Container, options)
	if errdefs.IsNotImplemented(err) {
		logrus.Warnf("Can't retrieve logs for %q: %s", getCanonicalContainerName(ctr), err.Error())
		return nil
	}
	return err
}

// followStartedContainersLogs streams the logs of containers (re)started
// while following, ignoring those whose logging driver doesn't support
// reading logs
func (s *composeService) followStartedContainersLogs(
	ctx context.Context,
	eg *errgroup.Group,
	limiter *semaphore.Weighted,
	consumer api.LogConsumer,
	options api.LogOptions,
) api.ContainerEventListener {
	return func(event api.ContainerEvent) {
		if event.Type != api.ContainerEventStarted {
			return
		}
		eg.Go(func() error {
			if err := acquireSlot(ctx, limiter); err != nil {
				return err
			}
			res, err := s.apiClient().ContainerInspect(ctx, event.ID, client.ContainerInspectOptions{})
			if err != nil {
				releaseSlot(limiter)
				return err
			}

			err = s.doLogContainer(ctx, limiter, consumer, event.Source, res.Container, api.LogOptions{
				Follow:     options.Follow,
				Since:      res.Container.State.StartedAt,
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
}

// doLogContainer opens the container's log stream and copies it to consumer
// until it ends. The caller must have already acquired limiter's slot (see
// acquireSlot); it is released here right after ContainerLogs returns, so a
// long-lived --follow stream never keeps blocking new connections or the
// monitor.
func (s *composeService) doLogContainer(ctx context.Context, limiter *semaphore.Weighted, consumer api.LogConsumer, name string, ctr container.InspectResponse, options api.LogOptions) error {
	r, err := s.apiClient().ContainerLogs(ctx, ctr.ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     options.Follow,
		Since:      options.Since,
		Until:      options.Until,
		Tail:       options.Tail,
		Timestamps: options.Timestamps,
	})
	releaseSlot(limiter)
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
