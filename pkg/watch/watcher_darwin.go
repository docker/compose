package watch

import (
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/windmilleng/tilt/internal/ospath"
	"github.com/windmilleng/tilt/pkg/logger"

	"github.com/windmilleng/fsevents"
)

// A file watcher optimized for Darwin.
// Uses FSEvents to avoid the terrible perf characteristics of kqueue.
type darwinNotify struct {
	stream *fsevents.EventStream
	events chan FileEvent
	errors chan error
	stop   chan struct{}

	pathsWereWatching map[string]interface{}
	ignore            PathMatcher
	logger            logger.Logger
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
					d.sawAnyHistoryDone = true
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

				ignore, err := d.ignore.Matches(e.Path)
				if err != nil {
					d.logger.Infof("Error matching path %q: %v", e.Path, err)
				} else if ignore {
					continue
				}

				d.events <- NewFileEvent(e.Path)
			}
		}
	}
}

// Add a path to be watched. Should only be called during initialization.
func (d *darwinNotify) initAdd(name string) {
	// Check if this is a subdirectory of any of the paths
	// we're already watching.
	for _, parent := range d.stream.Paths {
		if ospath.IsChild(parent, name) {
			return
		}
	}

	d.stream.Paths = append(d.stream.Paths, name)

	if d.pathsWereWatching == nil {
		d.pathsWereWatching = make(map[string]interface{})
	}
	d.pathsWereWatching[name] = struct{}{}
}

func (d *darwinNotify) Start() error {
	if len(d.stream.Paths) == 0 {
		return nil
	}

	numberOfWatches.Add(int64(len(d.stream.Paths)))

	d.stream.Start()

	go d.loop()

	return nil
}

func (d *darwinNotify) Close() error {
	numberOfWatches.Add(int64(-len(d.stream.Paths)))

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

func newWatcher(paths []string, ignore PathMatcher, l logger.Logger) (*darwinNotify, error) {
	dw := &darwinNotify{
		ignore: ignore,
		logger: l,
		stream: &fsevents.EventStream{
			Latency: 1 * time.Millisecond,
			Flags:   fsevents.FileEvents,
			// NOTE(dmiller): this corresponds to the `sinceWhen` parameter in FSEventStreamCreate
			// https://developer.apple.com/documentation/coreservices/1443980-fseventstreamcreate
			EventID: fsevents.LatestEventID(),
		},
		events: make(chan FileEvent),
		errors: make(chan error),
		stop:   make(chan struct{}),
	}

	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Wrap(err, "newWatcher")
		}
		dw.initAdd(path)
	}

	return dw, nil
}

var _ Notify = &darwinNotify{}
