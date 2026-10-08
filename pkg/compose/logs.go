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
	"bytes"
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

// inspectWithSlot acquires limiter's slot, then inspects the container,
// releasing the slot on error since no caller reaches doLogContainer (which
// owns the slot from here on) in that case.
func (s *composeService) inspectWithSlot(ctx context.Context, limiter *semaphore.Weighted, id string) (container.InspectResponse, error) {
	if err := acquireSlot(ctx, limiter); err != nil {
		return container.InspectResponse{}, err
	}
	defer panicSafeReleaseSlot(limiter)
	res, err := s.apiClient().ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		releaseSlot(limiter)
		return container.InspectResponse{}, err
	}
	return res.Container, nil
}

// logContainer streams a container's logs, warning when its logging driver
// doesn't support reading logs
func (s *composeService) logContainer(ctx context.Context, limiter *semaphore.Weighted, consumer api.LogConsumer, ctr container.Summary, options api.LogOptions) error {
	res, err := s.inspectWithSlot(ctx, limiter, ctr.ID)
	if err != nil {
		return err
	}
	err = s.doLogContainer(ctx, limiter, consumer, getContainerNameWithoutProject(ctr), res, options, nil)
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
	runEnds := newRunEndTracker()
	cursors := newLogCursors()
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
		// Queued here, in event order, for the same reason.
		turn := cursors.enter(event.ID)
		eg.Go(func() error {
			defer turn.end()
			if err := turn.wait(ctx); err != nil {
				return err
			}
			res, err := s.inspectWithSlot(ctx, limiter, event.ID)
			if err != nil {
				return err
			}
			if since == "" {
				since = logsSinceLastRun(res)
			}

			err = s.doLogContainer(ctx, limiter, consumer, event.Source, res, api.LogOptions{
				Follow:     options.Follow,
				Since:      since,
				Until:      options.Until,
				Tail:       options.Tail,
				Timestamps: options.Timestamps,
			}, turn.cursor)
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

// Observe records exit events (other event types are ignored). An exit
// carrying no timestamp is deliberately dropped rather than patched with the
// local clock: the anchor is compared by the DAEMON against its own
// container-log timestamps, so substituting our clock would trade a
// hypothetical daemon quirk for real clock-skew mis-anchoring. Dropping it
// merely degrades that container to the logsSinceLastRun fallback — the
// exact pre-tracker behavior, imperfect only for a run fast enough to have
// finished again by inspection time.
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

// doLogContainer opens the container's log stream and copies it to consumer
// until it ends. The caller must have already acquired limiter's slot (see
// acquireSlot); it is released here right after ContainerLogs returns, so a
// long-lived --follow stream never keeps blocking new connections or the
// monitor.
//
// cur, when not nil, is the container's logCursor: the stream then starts after
// the last line it relayed, and records the lines it relays. It is ignored for
// TTY containers, whose unframed stream cannot carry the timestamps it needs.
func (s *composeService) doLogContainer(
	ctx context.Context,
	limiter *semaphore.Weighted,
	consumer api.LogConsumer,
	name string,
	ctr container.InspectResponse,
	options api.LogOptions,
	cur *logCursor,
) error {
	if cur != nil && ctr.Config.Tty {
		cur = nil
	}
	keepTimestamps := options.Timestamps
	if cur != nil {
		options.Since = cur.since(options.Since)
		options.Timestamps = true
	}
	// Scoped to a closure so panicSafeReleaseSlot's defer only guards the
	// acquire-to-release window: releaseSlot below is unconditional once
	// ContainerLogs returns, so a panic during the copy loop that follows
	// must not re-trigger it and release the same slot twice.
	r, err := func() (io.ReadCloser, error) {
		defer panicSafeReleaseSlot(limiter)
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
		return r, err
	}()
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck

	w := utils.GetWriter(func(line string) {
		consumer.Log(name, line)
	})
	switch {
	case ctr.Config.Tty:
		_, err = io.Copy(w, r)
	case cur != nil:
		cw := &cursorWriter{cur: cur, keep: keepTimestamps, next: w}
		_, err = stdcopy.StdCopy(cw, cw, r)
	default:
		_, err = stdcopy.StdCopy(w, w, r)
	}
	return err
}

// logCursors hands the successive log streams of a container over to each
// other, so that a restarted container's lines are relayed exactly once.
//
// The log journal is shared by all the runs of a container, and a followed
// stream has no upper bound: it ends with the run in progress when it opened.
// Opened late, after the container restarted again (the restart delay is
// 100ms, a loaded daemon or monitor easily takes longer), it returns the next
// run's lines as well, which that run's own stream then returns a second time.
// So the streams of a container run one at a time, in start-event order, each
// starting right after the last line the previous ones relayed. Line
// timestamps are the daemon's own, no clock of ours is involved.
type logCursors struct {
	mu sync.Mutex
	by map[string]*logCursor
}

// logCursor is the position of a container's log relay.
type logCursor struct {
	tail chan struct{} // closed when the latest queued turn ends
	last time.Time     // newest timestamp relayed; only read or written by the turn holder
}

// logTurn is one stream's place in line for its container's relay.
type logTurn struct {
	cursor *logCursor
	prev   <-chan struct{}
	done   chan struct{}
	once   sync.Once
}

func newLogCursors() *logCursors {
	return &logCursors{by: map[string]*logCursor{}}
}

// enter queues a stream for container id. Turns are granted in the order
// enter is called: call it synchronously from the (ordered) event listener,
// not from the goroutine that will stream.
func (c *logCursors) enter(id string) *logTurn {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.by[id]
	if !ok {
		first := make(chan struct{})
		close(first)
		cur = &logCursor{tail: first}
		c.by[id] = cur
	}
	t := &logTurn{cursor: cur, prev: cur.tail, done: make(chan struct{})}
	cur.tail = t.done
	return t
}

// wait blocks until every earlier stream of the container has ended.
func (t *logTurn) wait(ctx context.Context) error {
	select {
	case <-t.prev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// end hands over to the next stream. Every turn must end, including those
// that never got to stream; it is safe to call more than once.
func (t *logTurn) end() {
	t.once.Do(func() { close(t.done) })
}

// since returns where the next stream must start: right after the last line
// already relayed, unless the anchor (RFC3339Nano, "" for none) is later, as it
// is for a stream opened in time: it then starts as it always did.
func (cur *logCursor) since(anchor string) string {
	if cur.last.IsZero() {
		return anchor
	}
	if t, err := time.Parse(time.RFC3339Nano, anchor); err == nil && t.After(cur.last) {
		return anchor
	}
	return cur.last.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano)
}

// cursorWriter records the timestamp of each log entry before relaying it. The
// stream was opened with timestamps, which the daemon puts in front of every
// entry (so also in the middle of a line longer than 16KiB, which it
// splits); each Write of the demultiplexed stream is one entry. They are
// removed unless keep, i.e. unless the caller asked for them.
type cursorWriter struct {
	cur  *logCursor
	keep bool
	next io.Writer
}

func (w *cursorWriter) Write(p []byte) (int, error) {
	out := p
	if ts, rest, ok := bytes.Cut(p, []byte{' '}); ok {
		if t, err := time.Parse(time.RFC3339Nano, string(ts)); err == nil {
			if t.After(w.cur.last) {
				w.cur.last = t
			}
			if !w.keep {
				out = rest
			}
		}
	}
	if _, err := w.next.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}
