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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// fakeLogJournal models what the daemon does with a container's json-file
// log: ONE journal for every run of the container, from which a followed
// stream returns the entries from `since` on, then the new ones until the run
// in progress ends. A stream opened between two runs, or after the container
// is over, returns the backlog and ends.
type fakeLogJournal struct {
	mu      sync.Mutex
	now     time.Time
	entries []fakeLogEntry
	running bool
	subs    []chan fakeLogEntry
}

type fakeLogEntry struct {
	ts     time.Time
	stream stdcopy.StdType
	text   string
}

func newFakeLogJournal() *fakeLogJournal {
	return &fakeLogJournal{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

// tick moves the clock forward and returns the new time, so that every event
// and every entry has its own timestamp.
func (j *fakeLogJournal) tick() time.Time {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.now = j.now.Add(time.Millisecond)
	return j.now
}

func (j *fakeLogJournal) startRun() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.running = true
}

func (j *fakeLogJournal) endRun() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.running = false
	for _, ch := range j.subs {
		close(ch)
	}
	j.subs = nil
}

func (j *fakeLogJournal) log(text string) {
	j.logAt(j.tick(), stdcopy.Stdout, text)
}

func (j *fakeLogJournal) logAt(ts time.Time, stream stdcopy.StdType, text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e := fakeLogEntry{ts: ts, stream: stream, text: text}
	j.entries = append(j.entries, e)
	for _, ch := range j.subs {
		ch <- e
	}
}

// open is ContainerLogs: the entries since `since` (inclusive, as the
// daemon's), with the daemon's timestamp in front of each when asked for. A
// line longer than 16KiB is split in as many entries, each prefixed, only the
// last one ending with a newline.
func (j *fakeLogJournal) open(since string, timestamps bool) io.ReadCloser {
	var from time.Time
	if since != "" {
		var err error
		from, err = time.Parse(time.RFC3339Nano, since)
		if err != nil {
			panic(err)
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	ch := make(chan fakeLogEntry, 1024)
	for _, e := range j.entries {
		if !e.ts.Before(from) {
			ch <- e
		}
	}
	if j.running {
		j.subs = append(j.subs, ch)
	} else {
		close(ch)
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close() //nolint:errcheck
		for e := range ch {
			if err := writeFakeLogEntry(pw, e, timestamps); err != nil {
				return
			}
		}
	}()
	return pr
}

func writeFakeLogEntry(w io.Writer, e fakeLogEntry, timestamps bool) error {
	out := newStdWriter(w, e.stream)
	for rest := e.text; ; {
		chunk, last := rest, true
		if len(rest) > 16*1024 {
			chunk, rest, last = rest[:16*1024], rest[16*1024:], false
		}
		if last {
			chunk += "\n"
		}
		if timestamps {
			chunk = e.ts.UTC().Format("2006-01-02T15:04:05.000000000Z07:00") + " " + chunk
		}
		if _, err := out.Write([]byte(chunk)); err != nil || last {
			return err
		}
	}
}

// fakeLogDaemon is the API side: the journal, behind ContainerInspect and
// ContainerLogs, and a record of how the streams were opened.
type fakeLogDaemon struct {
	journal *fakeLogJournal
	gate    chan struct{} // ContainerLogs blocks until it is closed
	tty     bool
	failing map[int]error // opening of the n-th stream (1-based) fails

	mu    sync.Mutex
	opens []client.ContainerLogsOptions
}

func newFakeLogDaemon(apiClient *mocks.MockAPIClient) *fakeLogDaemon {
	d := &fakeLogDaemon{journal: newFakeLogJournal(), gate: make(chan struct{}), failing: map[int]error{}}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{Container: containerType.InspectResponse{
				ID:     "c1",
				Config: &containerType.Config{Tty: d.tty},
				State:  &containerType.State{},
			}}, nil
		}).AnyTimes()
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "c1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.ContainerLogsOptions) (io.ReadCloser, error) {
			d.mu.Lock()
			d.opens = append(d.opens, opts)
			n := len(d.opens)
			d.mu.Unlock()
			<-d.gate
			if err := d.failing[n]; err != nil {
				return nil, err
			}
			if d.tty {
				return io.NopCloser(strings.NewReader("tty line\r\n")), nil
			}
			return d.journal.open(opts.Since, opts.Timestamps), nil
		}).AnyTimes()
	return d
}

func (d *fakeLogDaemon) openedStreams() []client.ContainerLogsOptions {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]client.ContainerLogsOptions(nil), d.opens...)
}

// play drives a container through its runs, delivering the events the monitor
// would. The first run is the one `up` attached to, outside of the log
// streams; each later one is a restart. With late, the daemon only answers
// the streams once every run is over: each stream is opened after the
// container restarted again, as a slow monitor or a loaded daemon makes it.
func (d *fakeLogDaemon) play(t *testing.T, listener api.ContainerEventListener, late bool, runs ...func(j *fakeLogJournal)) {
	t.Helper()
	event := func(typ int, restarting bool) {
		listener(api.ContainerEvent{
			Type: typ, ID: "c1", Source: "failing-1", Service: "failing",
			Time: d.journal.tick().UnixNano(), Restarting: restarting,
		})
	}
	if !late {
		close(d.gate)
	}
	for i, run := range runs {
		d.journal.startRun()
		if i > 0 {
			event(api.ContainerEventStarted, true)
			if !late {
				waitUntil(t, func() bool { return len(d.openedStreams()) >= i }, "stream %d must open", i)
			}
		}
		run(d.journal)
		d.journal.endRun()
		event(api.ContainerEventExited, false)
	}
	if late {
		close(d.gate)
	}
}

func waitUntil(t *testing.T, cond func() bool, msg string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: "+msg, args...)
		}
		time.Sleep(time.Millisecond)
	}
}

func logs(texts ...string) func(*fakeLogJournal) {
	return func(j *fakeLogJournal) {
		for _, text := range texts {
			j.log(text)
		}
	}
}

// relay is one of the two places that follow the restarts of a container.
type relay struct {
	name  string
	start func(t *testing.T, svc *composeService, consumer api.LogConsumer, opts api.LogOptions) (api.ContainerEventListener, func() error)
}

var relays = []relay{
	{"logs --follow", func(t *testing.T, svc *composeService, consumer api.LogConsumer, opts api.LogOptions) (api.ContainerEventListener, func() error) {
		var eg errgroup.Group
		return svc.followStartedContainersLogs(t.Context(), &eg, nil, consumer, opts), eg.Wait
	}},
	{"up", func(t *testing.T, svc *composeService, consumer api.LogConsumer, _ api.LogOptions) (api.ContainerEventListener, func() error) {
		u := &upSession{
			composeService: svc,
			globalCtx:      t.Context(),
			options:        api.UpOptions{Start: api.StartOptions{Attach: consumer}},
		}
		return u.followStartedContainers(nil), func() error {
			_ = u.eg.Wait()
			return errors.Join(u.errs...)
		}
	}},
}

func finish(t *testing.T, wait func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("log streams did not finish: a stream is blocked")
		return nil
	}
}

// TestRestartedContainerLogsAreRelayedOnce is the reported flake: a container
// that restarts twice, a line logged by each run, and each line must be shown
// once even when a stream opens after the container restarted again (then it
// returns the next run's lines too, which the next stream must not repeat).
func TestRestartedContainerLogsAreRelayedOnce(t *testing.T) {
	tests := []struct {
		name string
		late bool
		runs []func(*fakeLogJournal)
		want []string
	}{
		{"streams opened late", true, []func(*fakeLogJournal){logs("world"), logs("world"), logs("world")}, []string{"world", "world"}},
		{"streams opened in time", false, []func(*fakeLogJournal){logs("world"), logs("world"), logs("world")}, []string{"world", "world"}},
		{"later runs never log", true, []func(*fakeLogJournal){logs("a"), logs(), logs()}, nil},
		{"a run without log in the middle", true, []func(*fakeLogJournal){logs("a"), logs(), logs("c")}, []string{"c"}},
		{"many restarts, late", true, []func(*fakeLogJournal){logs("1"), logs("2"), logs("3"), logs("4"), logs("5")}, []string{"2", "3", "4", "5"}},
		{"many restarts, in time", false, []func(*fakeLogJournal){logs("1"), logs("2"), logs("3"), logs("4"), logs("5")}, []string{"2", "3", "4", "5"}},
		{"several lines per run", true, []func(*fakeLogJournal){logs("1"), logs("2a", "2b"), logs("3a", "3b")}, []string{"2a", "2b", "3a", "3b"}},
		{"stdout and stderr out of order", true, []func(*fakeLogJournal){logs("1"), logs("2"), func(j *fakeLogJournal) {
			// the daemon timestamps each stream by itself: the entry the journal
			// ends with is not the newest one
			ts := j.tick()
			j.logAt(ts.Add(5*time.Microsecond), stdcopy.Stdout, "out")
			j.logAt(ts, stdcopy.Stderr, "err")
		}, logs()}, []string{"2", "out", "err"}},
	}
	for _, r := range relays {
		for _, tt := range tests {
			t.Run(r.name+"/"+tt.name, func(t *testing.T) {
				svc, apiClient := newTestService(t)
				d := newFakeLogDaemon(apiClient)
				consumer := &testLogConsumer{}
				listener, wait := r.start(t, svc, consumer, api.LogOptions{Follow: true})

				d.play(t, listener, tt.late, tt.runs...)
				assert.NilError(t, finish(t, wait))
				assert.DeepEqual(t, consumer.LogsForContainer("failing-1"), tt.want)
			})
		}
	}
}

// TestRestartedContainerLogs_Timestamps checks the timestamps the streams ask
// the daemon for to know where they are never reach the output, unless the
// caller asked for them, in which case they come out as the daemon wrote them:
// also in the middle of a line longer than 16KiB, which it splits.
func TestRestartedContainerLogs_Timestamps(t *testing.T) {
	long := strings.Repeat("a", 40000)
	runs := []func(*fakeLogJournal){logs("1"), logs("two", long), logs("three")}

	for _, r := range relays {
		t.Run(r.name+"/not requested", func(t *testing.T) {
			svc, apiClient := newTestService(t)
			d := newFakeLogDaemon(apiClient)
			consumer := &testLogConsumer{}
			listener, wait := r.start(t, svc, consumer, api.LogOptions{Follow: true})
			d.play(t, listener, true, runs...)
			assert.NilError(t, finish(t, wait))
			assert.DeepEqual(t, consumer.LogsForContainer("failing-1"), []string{"two", long, "three"})
		})
	}

	t.Run("logs --follow --timestamps", func(t *testing.T) {
		svc, apiClient := newTestService(t)
		d := newFakeLogDaemon(apiClient)
		consumer := &testLogConsumer{}
		listener, wait := relays[0].start(t, svc, consumer, api.LogOptions{Follow: true, Timestamps: true})
		d.play(t, listener, true, runs...)
		assert.NilError(t, finish(t, wait))

		const prefix = len("2026-01-02T03:04:05.000000000Z ")
		got := consumer.LogsForContainer("failing-1")
		assert.Equal(t, len(got), 3, "%v", got)
		for _, line := range got {
			assert.Assert(t, strings.HasPrefix(line, "2026-01-02T03:04:05."), line)
		}
		assert.Equal(t, got[0][prefix:], "two")
		assert.Equal(t, got[2][prefix:], "three")
		// as `docker logs --timestamps`: one prefix per 16KiB entry
		assert.Equal(t, len(got[1]), len(long)+3*prefix)
	})
}

// TestRestartedContainerLogs_StreamOptions checks what the streams ask the
// daemon for: the caller's options are kept; a stream opened in time starts at
// the previous run's end, as it always did (which is all the daemons whose
// timestamps are coarser than a nanosecond can be asked); one that finds the
// previous streams went past that end starts right after the last line they
// relayed.
func TestRestartedContainerLogs_StreamOptions(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	at := func(d time.Duration) string { return base.Add(d).Format(time.RFC3339Nano) }

	tests := []struct {
		name  string
		late  bool
		since []string
	}{
		// ticks: "1" at +1ms, its exit +2ms, run 2's start +3ms, "2" +4ms, exit +5ms,
		// run 3's start +6ms, "3" +7ms, exit +8ms, run 4's start +9ms, exit +10ms
		{"in time", false, []string{at(2 * time.Millisecond), at(5 * time.Millisecond), at(8 * time.Millisecond)}},
		// the first stream relays up to "3": the second one starts right after it,
		// the third one, with nothing newer relayed since the run it follows, at its end
		{"late", true, []string{at(2 * time.Millisecond), at(7*time.Millisecond + time.Nanosecond), at(8 * time.Millisecond)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, apiClient := newTestService(t)
			d := newFakeLogDaemon(apiClient)
			consumer := &testLogConsumer{}
			listener, wait := relays[0].start(t, svc, consumer, api.LogOptions{Follow: true, Until: "2030-01-01T00:00:00Z", Tail: "7"})
			d.play(t, listener, tt.late, logs("1"), logs("2"), logs("3"), logs())
			assert.NilError(t, finish(t, wait))

			opens := d.openedStreams()
			assert.Equal(t, len(opens), 3)
			for i, o := range opens {
				assert.Equal(t, o.Since, tt.since[i], "stream %d", i+1)
				assert.Assert(t, o.Follow && o.ShowStdout && o.ShowStderr)
				assert.Equal(t, o.Until, "2030-01-01T00:00:00Z")
				assert.Equal(t, o.Tail, "7")
				assert.Assert(t, o.Timestamps, "the streams need the daemon's timestamps")
			}
			assert.DeepEqual(t, consumer.LogsForContainer("failing-1"), []string{"2", "3"})
		})
	}
}

// TestRestartedContainerLogs_TTY: a TTY container's stream is not multiplexed,
// so it cannot carry the timestamps the cursor reads: such a container is
// followed as it always was.
func TestRestartedContainerLogs_TTY(t *testing.T) {
	for _, r := range relays {
		t.Run(r.name, func(t *testing.T) {
			svc, apiClient := newTestService(t)
			d := newFakeLogDaemon(apiClient)
			d.tty = true
			consumer := &testLogConsumer{}
			listener, wait := r.start(t, svc, consumer, api.LogOptions{Follow: true})
			d.play(t, listener, false, logs("1"), logs("2"), logs("3"))
			assert.NilError(t, finish(t, wait))

			opens := d.openedStreams()
			assert.Equal(t, len(opens), 2)
			for _, o := range opens {
				assert.Assert(t, !o.Timestamps)
			}
			base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			assert.Equal(t, opens[0].Since, base.Add(2*time.Millisecond).Format(time.RFC3339Nano), "the previous run's end, as always")
			assert.Equal(t, opens[1].Since, base.Add(5*time.Millisecond).Format(time.RFC3339Nano))
			assert.DeepEqual(t, consumer.LogsForContainer("failing-1"), []string{"tty line\r", "tty line\r"})
		})
	}
}

// TestRestartedContainerLogs_FailedStream: a stream that cannot be opened must
// not hold the following ones back, and they start where it would have.
func TestRestartedContainerLogs_FailedStream(t *testing.T) {
	for _, r := range relays {
		t.Run(r.name, func(t *testing.T) {
			svc, apiClient := newTestService(t)
			d := newFakeLogDaemon(apiClient)
			d.failing[1] = errors.New("boom")
			consumer := &testLogConsumer{}
			listener, wait := r.start(t, svc, consumer, api.LogOptions{Follow: true})
			d.play(t, listener, false, logs("1"), logs("2"), logs("3"))
			err := finish(t, wait)
			assert.ErrorContains(t, err, "boom")
			assert.DeepEqual(t, consumer.LogsForContainer("failing-1"), []string{"3"})
		})
	}
}

// TestLogCursors_Turns checks the hand-over between the streams of a
// container: in the order they were queued, whichever goroutine gets there
// first; and a stream that gives up (canceled context, error, panic) never
// holds the others back.
func TestLogCursors_Turns(t *testing.T) {
	cursors := newLogCursors()
	first, second, third := cursors.enter("c1"), cursors.enter("c1"), cursors.enter("c1")
	other := cursors.enter("c2")

	granted := make(chan string, 3)
	var wg sync.WaitGroup
	for name, turn := range map[string]*logTurn{"third": third, "second": second} { // later turns ask first
		wg.Go(func() {
			assert.NilError(t, turn.wait(t.Context()))
			granted <- name
			turn.end()
		})
	}

	assert.NilError(t, other.wait(t.Context()), "another container's streams are independent")
	select {
	case name := <-granted:
		t.Fatalf("%s got its turn before the first stream ended", name)
	case <-time.After(50 * time.Millisecond):
	}

	assert.NilError(t, first.wait(t.Context()))
	first.end()
	first.end() // harmless
	wg.Wait()
	assert.Equal(t, <-granted, "second")
	assert.Equal(t, <-granted, "third")
}

func TestLogCursors_CanceledWaiterReleasesNext(t *testing.T) {
	cursors := newLogCursors()
	first, second, third := cursors.enter("c1"), cursors.enter("c1"), cursors.enter("c1")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, second.wait(ctx), context.Canceled)
	second.end() // what the stream's defer does

	first.end()
	assert.NilError(t, third.wait(t.Context()))
}
