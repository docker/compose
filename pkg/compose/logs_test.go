/*
   Copyright 2022 Docker Compose CLI authors

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
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	compose "github.com/docker/compose/v5/pkg/api"
)

// newStdWriter is copied from github.com/moby/moby/daemon/internal/stdcopymux
// because NewStdWriter was moved to a daemon-internal package in moby v2 and
// is no longer publicly importable. We need it in tests to produce multiplexed
// streams that stdcopy.StdCopy can demultiplex.

const (
	stdWriterPrefixLen = 8
	stdWriterFdIndex   = 0
	stdWriterSizeIndex = 4
)

var bufPool = &sync.Pool{New: func() any { return bytes.NewBuffer(nil) }}

type stdWriter struct {
	io.Writer
	prefix byte
}

func (w *stdWriter) Write(p []byte) (int, error) {
	if w == nil || w.Writer == nil {
		return 0, errors.New("writer not instantiated")
	}
	if p == nil {
		return 0, nil
	}

	header := [stdWriterPrefixLen]byte{stdWriterFdIndex: w.prefix}
	binary.BigEndian.PutUint32(header[stdWriterSizeIndex:], uint32(len(p)))
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Write(header[:])
	buf.Write(p)

	n, err := w.Writer.Write(buf.Bytes())
	n -= stdWriterPrefixLen
	if n < 0 {
		n = 0
	}

	buf.Reset()
	bufPool.Put(buf)
	return n, err
}

func newStdWriter(w io.Writer, streamType stdcopy.StdType) io.Writer {
	return &stdWriter{
		Writer: w,
		prefix: byte(streamType),
	}
}

func TestComposeService_Logs_Demux(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		All:     true,
		Filters: projectFilter(name).Add("label", oneOffFilter(false), compose.ConfigHashLabel),
	}).Return(
		client.ContainerListResult{
			Items: []containerType.Summary{
				testContainer("service", "c", false),
			},
		},
		nil,
	)

	api.EXPECT().
		ContainerInspect(anyCancellableContext(), "c", gomock.Any()).
		Return(client.ContainerInspectResult{
			Container: containerType.InspectResponse{
				ID:     "c",
				Config: &containerType.Config{Tty: false},
			},
		}, nil)
	c1Reader, c1Writer := io.Pipe()
	t.Cleanup(func() {
		_ = c1Reader.Close()
		_ = c1Writer.Close()
	})
	c1Stdout := newStdWriter(c1Writer, stdcopy.Stdout)
	c1Stderr := newStdWriter(c1Writer, stdcopy.Stderr)
	go func() {
		_, err := c1Stdout.Write([]byte("hello stdout\n"))
		assert.NilError(t, err, "Writing to fake stdout")
		_, err = c1Stderr.Write([]byte("hello stderr\n"))
		assert.NilError(t, err, "Writing to fake stderr")
		_ = c1Writer.Close()
	}()
	api.EXPECT().ContainerLogs(anyCancellableContext(), "c", gomock.Any()).
		Return(c1Reader, nil)

	opts := compose.LogOptions{
		Project: &types.Project{
			Services: types.Services{
				"service": {Name: "service"},
			},
		},
	}

	consumer := &testLogConsumer{}
	err = tested.Logs(t.Context(), name, consumer, opts)
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{"hello stdout", "hello stderr"}, consumer.LogsForContainer("c"))
}

// TestComposeService_Logs_ServiceFiltering ensures that we do not include
// logs from out-of-scope services based on the Compose file vs actual state.
//
// NOTE(milas): This test exists because each method is currently duplicating
// a lot of the project/service filtering logic. We should consider moving it
// to an earlier point in the loading process, at which point this test could
// safely be removed.
func TestComposeService_Logs_ServiceFiltering(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		All:     true,
		Filters: projectFilter(name).Add("label", oneOffFilter(false), compose.ConfigHashLabel),
	}).Return(
		client.ContainerListResult{
			Items: []containerType.Summary{
				testContainer("serviceA", "c1", false),
				testContainer("serviceA", "c2", false),
				// serviceB will be filtered out by the project definition to
				// ensure we ignore "orphan" containers
				testContainer("serviceB", "c3", false),
				testContainer("serviceC", "c4", false),
			},
		},
		nil,
	)

	for _, id := range []string{"c1", "c2", "c4"} {
		api.EXPECT().
			ContainerInspect(anyCancellableContext(), id, gomock.Any()).
			Return(
				client.ContainerInspectResult{
					Container: containerType.InspectResponse{
						ID:     id,
						Config: &containerType.Config{Tty: true},
					},
				},
				nil,
			)
		api.EXPECT().ContainerLogs(anyCancellableContext(), id, gomock.Any()).
			Return(io.NopCloser(strings.NewReader("hello "+id+"\n")), nil).
			Times(1)
	}

	// this simulates passing `--filename` with a Compose file that does NOT
	// reference `serviceB` even though it has running services for this proj
	proj := &types.Project{
		Services: types.Services{
			"serviceA": {Name: "serviceA"},
			"serviceC": {Name: "serviceC"},
		},
	}
	consumer := &testLogConsumer{}
	opts := compose.LogOptions{
		Project: proj,
	}
	err = tested.Logs(t.Context(), name, consumer, opts)
	assert.NilError(t, err)

	assert.Assert(t, is.DeepEqual([]string{"hello c1"}, consumer.LogsForContainer("c1")))
	assert.Assert(t, is.DeepEqual([]string{"hello c2"}, consumer.LogsForContainer("c2")))
	assert.Assert(t, is.Len(consumer.LogsForContainer("c3"), 0))
	assert.Assert(t, is.DeepEqual([]string{"hello c4"}, consumer.LogsForContainer("c4")))
}

// TestComposeService_Logs_FollowDoesNotStarveMonitor guards against a
// regression where bounding the errgroup used for `--follow` log streams
// (which never return) starved the monitor goroutine, submitted after them
// to the same group: once maxConcurrency streams were open, the monitor
// never started and later containers never appeared.
func TestComposeService_Logs_FollowDoesNotStarveMonitor(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli, WithMaxConcurrency(1))
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{
			Items: []containerType.Summary{
				testContainer("service", "c1", false),
				testContainer("service", "c2", false),
			},
		}, nil).
		Times(2) // selectLogsContainers, then the monitor's initialContainers

	writers := make(map[string]*io.PipeWriter)
	// opened only fires once a container's stream is actually established
	// (post-semaphore, mid-copy): closing writers before that would race
	// with the semaphore-gated open itself, closing a pipe nothing reads yet.
	opened := make(chan struct{}, 2)
	for _, id := range []string{"c1", "c2"} {
		r, w := io.Pipe()
		writers[id] = w
		t.Cleanup(func() { _ = r.Close() })

		api.EXPECT().
			ContainerInspect(anyCancellableContext(), id, gomock.Any()).
			Return(client.ContainerInspectResult{
				Container: containerType.InspectResponse{
					ID:     id,
					Config: &containerType.Config{Tty: false},
				},
			}, nil)
		// never closed on its own: simulates a `--follow` stream that stays
		// open for the container's lifetime
		api.EXPECT().ContainerLogs(anyCancellableContext(), id, gomock.Any()).
			DoAndReturn(func(context.Context, string, client.ContainerLogsOptions) (io.ReadCloser, error) {
				opened <- struct{}{}
				return r, nil
			})
	}

	started := make(chan struct{})
	api.EXPECT().Events(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ client.EventsListOptions) client.EventsResult {
			close(started)
			return client.EventsResult{Messages: make(chan events.Message), Err: make(chan error)}
		})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tested.Logs(ctx, name, &testLogConsumer{}, compose.LogOptions{Follow: true})
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor never started: follow-mode log streams starved the bounded errgroup")
	}
	for range 2 {
		select {
		case <-opened:
		case <-time.After(5 * time.Second):
			t.Fatal("not every container's log stream opened")
		}
	}

	cancel()
	for _, w := range writers {
		_ = w.Close()
	}
	assert.NilError(t, <-done)
}

// TestComposeService_Logs_FollowLimitsConcurrentStreamOpens guards against a
// regression where removing the concurrency bound entirely for `--follow`
// (to stop it starving the monitor, see
// TestComposeService_Logs_FollowDoesNotStarveMonitor) let an unbounded
// number of ContainerLogs calls open at once. Opening a stream must still
// respect maxConcurrency; only the (indefinite) copy that follows is exempt.
func TestComposeService_Logs_FollowLimitsConcurrentStreamOpens(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli, WithMaxConcurrency(1))
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	ids := []string{"c1", "c2", "c3"}
	var containerItems []containerType.Summary
	for _, id := range ids {
		containerItems = append(containerItems, testContainer("service", id, false))
	}
	api.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: containerItems}, nil).
		Times(2) // selectLogsContainers, then the monitor's initialContainers

	for _, id := range ids {
		api.EXPECT().
			ContainerInspect(anyCancellableContext(), id, gomock.Any()).
			Return(client.ContainerInspectResult{
				Container: containerType.InspectResponse{
					ID:     id,
					Config: &containerType.Config{Tty: false},
				},
			}, nil)
	}

	tracker := &peakConcurrencyTracker{}
	writers := make(chan *io.PipeWriter, len(ids))
	for _, id := range ids {
		api.EXPECT().ContainerLogs(anyCancellableContext(), id, gomock.Any()).
			DoAndReturn(func(context.Context, string, client.ContainerLogsOptions) (io.ReadCloser, error) {
				tracker.enter()
				time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
				tracker.leave()

				r, w := io.Pipe()
				writers <- w
				return r, nil
			})
	}

	started := make(chan struct{})
	api.EXPECT().Events(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ client.EventsListOptions) client.EventsResult {
			close(started)
			return client.EventsResult{Messages: make(chan events.Message), Err: make(chan error)}
		})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tested.Logs(ctx, name, &testLogConsumer{}, compose.LogOptions{Follow: true})
	}()

	<-started
	var openedWriters []*io.PipeWriter
	for range ids {
		select {
		case w := <-writers:
			openedWriters = append(openedWriters, w)
		case <-time.After(5 * time.Second):
			t.Fatal("not all follow-mode streams opened: maxConcurrency must bound the open burst, not the whole call")
		}
	}

	cancel()
	for _, w := range openedWriters {
		_ = w.Close()
	}
	assert.NilError(t, <-done)
	assert.Equal(t, tracker.Peak(), 1, "opening follow-mode log streams must be bounded by maxConcurrency")
}

type testLogConsumer struct {
	mu sync.Mutex
	// logs is keyed by container ID; values are log lines
	logs map[string][]string
}

func (l *testLogConsumer) Log(containerName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logs == nil {
		l.logs = make(map[string][]string)
	}
	l.logs[containerName] = append(l.logs[containerName], message)
}

func (l *testLogConsumer) Err(containerName, message string) {
	l.Log(containerName, message)
}

func (l *testLogConsumer) Status(containerName, msg string) {}

func (l *testLogConsumer) LogsForContainer(containerName string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.logs[containerName]
}

// TestRunEndTrackerAnchorsOnPreviousRun pins the re-attach anchor against the
// fast-run race seen in CI: with events delivered in order, the anchor
// captured at start-event time is the PREVIOUS run's end — even when the new
// run exits (and is observed) before the log stream is actually opened.
func TestRunEndTrackerAnchorsOnPreviousRun(t *testing.T) {
	tr := newRunEndTracker()

	// First start: no previous exit observed → no anchor (caller falls back
	// to the inspected FinishedAt).
	assert.Equal(t, tr.Since("c1"), "")

	// Run N exits at t=1_000_000_001ns, run N+1 starts: the anchor captured
	// at start-event time is run N's end, nanosecond-precise.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c1", Time: 1_000_000_001})
	anchor := tr.Since("c1")
	assert.Equal(t, anchor, "1970-01-01T00:00:01.000000001Z")

	// Run N+1 is fast: its own exit is observed before the log stream opens.
	// The anchor captured above must NOT move — reading it after this point
	// would exclude everything run N+1 printed.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c1", Time: 2_000_000_002})
	assert.Equal(t, anchor, "1970-01-01T00:00:01.000000001Z")
	// The NEXT start anchors on run N+1's end.
	assert.Equal(t, tr.Since("c1"), "1970-01-01T00:00:02.000000002Z")

	// Non-exit events and other containers do not pollute the anchor.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventStarted, ID: "c1", Time: 9_000_000_000})
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c2", Time: 3_000_000_003})
	assert.Equal(t, tr.Since("c1"), "1970-01-01T00:00:02.000000002Z")
}
