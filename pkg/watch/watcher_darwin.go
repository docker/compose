package watch

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/windmilleng/fsevents"
)

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

	// ignore the first event that says the watched directory
	// has been created. these are fired spuriously on initiation.
	ignoreCreatedEvents map[string]bool
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

				if e.Flags&fsevents.ItemCreated == fsevents.ItemCreated {
					d.sm.Lock()
					shouldIgnore := d.ignoreCreatedEvents[e.Path]
					if shouldIgnore {
						d.ignoreCreatedEvents[e.Path] = false
					} else {
						// If we got a created event for something
						// that's not on the ignore list, we assume
						// we're done with the spurious events.
						d.ignoreCreatedEvents = nil
					}
					d.sm.Unlock()

					if shouldIgnore {
						continue
					}
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

	if d.ignoreCreatedEvents == nil {
		d.ignoreCreatedEvents = make(map[string]bool, 1)
	}
	d.ignoreCreatedEvents[name] = true

	if len(es.Paths) == 1 {
		go d.loop()
		es.Start()
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
		},
		sm:     &sync.Mutex{},
		events: make(chan FileEvent),
		errors: make(chan error),
		stop:   make(chan struct{}),
	}

	return dw, nil
}

var _ Notify = &darwinNotify{}
