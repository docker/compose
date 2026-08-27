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
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

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

	eg, ctx := errgroup.WithContext(ctx)
	for _, ctr := range containers {
		eg.Go(func() error {
			return s.logContainer(ctx, consumer, ctr, options)
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
		monitor.withListener(s.followStartedContainersLogs(ctx, eg, consumer, options))
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
func (s *composeService) logContainer(ctx context.Context, consumer api.LogConsumer, ctr container.Summary, options api.LogOptions) error {
	res, err := s.apiClient().ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	err = s.doLogContainer(ctx, consumer, getContainerNameWithoutProject(ctr), res.Container, options)
	if errdefs.IsNotImplemented(err) {
		logrus.Warnf("Can't retrieve logs for %q: %s", getCanonicalContainerName(ctr), err.Error())
		return nil
	}
	return err
}

// followStartedContainersLogs streams the logs of containers (re)started
// while following, ignoring those whose logging driver doesn't support
// reading logs
func (s *composeService) followStartedContainersLogs(ctx context.Context, eg *errgroup.Group, consumer api.LogConsumer, options api.LogOptions) api.ContainerEventListener {
	runEnds := newRunEndTracker()
	return func(event api.ContainerEvent) {
		runEnds.Observe(event)
		if event.Type != api.ContainerEventStarted {
			return
		}
		// Captured synchronously: the monitor delivers events in order, so
		// the recorded end cannot yet include THIS run's own exit — reading
		// it inside the goroutine below could (fast run), and the window
		// would drop the whole run.
		since := runEnds.Since(event.ID)
		eg.Go(func() error {
			res, err := s.apiClient().ContainerInspect(ctx, event.ID, client.ContainerInspectOptions{})
			if err != nil {
				return err
			}
			if since == "" {
				since = logsSinceLastRun(res.Container)
			}

			err = s.doLogContainer(ctx, consumer, event.Source, res.Container, api.LogOptions{
				Follow:     options.Follow,
				Since:      since,
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

// runEndTracker remembers, per container, when the session last saw it exit —
// the re-attach anchor that stays correct even when the NEW run is already
// over: the event stream is ordered, so at start-event time the recorded
// value is necessarily the PREVIOUS run's end. The inspected FinishedAt
// (logsSinceLastRun) cannot give that guarantee — by the time we inspect, a
// fast run's own FinishedAt has overwritten it and the window would exclude
// everything the run printed.
type runEndTracker struct {
	mu   sync.Mutex
	ends map[string]int64 // container ID → TimeNano of the last observed exit
}

func newRunEndTracker() *runEndTracker {
	return &runEndTracker{ends: map[string]int64{}}
}

// Observe records exit events (other event types are ignored).
func (t *runEndTracker) Observe(e api.ContainerEvent) {
	if e.Type != api.ContainerEventExited || e.Time == 0 {
		return
	}
	t.mu.Lock()
	t.ends[e.ID] = e.Time
	t.mu.Unlock()
}

// Since returns the log-window anchor for a container being re-attached: the
// recorded end of its previous run in RFC3339Nano — the same format the
// FinishedAt fallback feeds the logs API — or "" when the session never saw
// it exit (first start).
func (t *runEndTracker) Since(containerID string) string {
	t.mu.Lock()
	nano, ok := t.ends[containerID]
	t.mu.Unlock()
	if !ok {
		return ""
	}
	return time.Unix(0, nano).UTC().Format(time.RFC3339Nano)
}

// logsSinceLastRun returns the FALLBACK log window anchor for a container
// (re)started while we follow the project, used when the session has not
// observed a previous exit (runEndTracker): the previous run's FinishedAt.
// The new run's StartedAt looks like the natural anchor but loses output —
// the daemon starts copying stdout before it records StartedAt, so a fast
// process can get its first lines timestamped just before it, and
// `since=StartedAt` then drops them forever. Nothing can be logged between
// the previous run's end and the new run's start, so FinishedAt captures the
// entire new run without replaying the previous one — UNLESS the new run
// already finished by inspection time (its own FinishedAt shadows the
// previous run's), which is exactly what the tracker protects against. A
// container with no previous run has a zero FinishedAt, which means "no
// lower bound" — equally exact for a fresh container.
func logsSinceLastRun(ctr container.InspectResponse) string {
	finished := ctr.State.FinishedAt
	if t, err := time.Parse(time.RFC3339Nano, finished); err != nil || t.Unix() <= 0 {
		return ""
	}
	return finished
}

func (s *composeService) doLogContainer(ctx context.Context, consumer api.LogConsumer, name string, ctr container.InspectResponse, options api.LogOptions) error {
	r, err := s.apiClient().ContainerLogs(ctx, ctr.ID, client.ContainerLogsOptions{
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
