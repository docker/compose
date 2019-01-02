package watch

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/windmilleng/fsevents"
)

// A file watcher optimized for Darwin.
// Uses FSEvents to avoid the terrible perf characteristics of kqueue.
type darwinNotify struct {
	stream *fsevents.EventStream
	events chan FileEvent
	errors chan error
	stop   chan struct{}

	// TODO(nick): This mutex is needed for the case where we add paths after we
	// start watching. But because fsevents supports recursive watches, we don't
	// actually need this feature. We should change the api contract of wmNotify
	// so that, for recursive watches, we can guarantee that the path list doesn't
	// change.
	sm *sync.Mutex

	pathsWereWatching map[string]interface{}
	sawAnyHistoryDone bool
}

func (d *darwinNotify) loop() {
	for {
		select {
		case <-d.stop:
			return
		case events, ok := <-d.stream.Events:
			if !ok {
				return
			}

			for _, e := range events {
				e.Path = filepath.Join("/", e.Path)

				if e.Flags&fsevents.HistoryDone == fsevents.HistoryDone {
					d.sm.Lock()
					d.sawAnyHistoryDone = true
					d.sm.Unlock()
					continue
				}

				// We wait until we've seen the HistoryDone event for this watcher before processing any events
				// so that we skip all of the "spurious" events that precede it.
				if !d.sawAnyHistoryDone {
					continue
				}

				_, isPathWereWatching := d.pathsWereWatching[e.Path]
				if e.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir && e.Flags&fsevents.ItemCreated == fsevents.ItemCreated && isPathWereWatching {
					// This is the first create for the path that we're watching. We always get exactly one of these
					// even after we get the HistoryDone event. Skip it.
					continue
				}

				d.events <- FileEvent{
					Path: e.Path,
				}
			}
		}
	}
}

func (d *darwinNotify) Add(name string) error {
	d.sm.Lock()
	defer d.sm.Unlock()

	es := d.stream

	// Check if this is a subdirectory of any of the paths
	// we're already watching.
	for _, parent := range es.Paths {
		isChild := pathIsChildOf(name, parent)
		if isChild {
			return nil
		}
	}

	es.Paths = append(es.Paths, name)

	if d.pathsWereWatching == nil {
		d.pathsWereWatching = make(map[string]interface{})
	}
	d.pathsWereWatching[name] = struct{}{}

	if len(es.Paths) == 1 {
		es.Start()
		go d.loop()
	} else {
		es.Restart()
	}

	return nil
}

func (d *darwinNotify) Close() error {
	d.sm.Lock()
	defer d.sm.Unlock()

	d.stream.Stop()
	close(d.errors)
	close(d.stop)

	return nil
}

func (d *darwinNotify) Events() chan FileEvent {
	return d.events
}

func (d *darwinNotify) Errors() chan error {
	return d.errors
}

func NewWatcher() (Notify, error) {
	dw := &darwinNotify{
		stream: &fsevents.EventStream{
			Latency: 1 * time.Millisecond,
			Flags:   fsevents.FileEvents,
			// NOTE(dmiller): this corresponds to the `sinceWhen` parameter in FSEventStreamCreate
			// https://developer.apple.com/documentation/coreservices/1443980-fseventstreamcreate
			EventID: fsevents.LatestEventID(),
		},
		sm:     &sync.Mutex{},
		events: make(chan FileEvent),
		errors: make(chan error),
		stop:   make(chan struct{}),
	}

	return dw, nil
}

var _ Notify = &darwinNotify{}
