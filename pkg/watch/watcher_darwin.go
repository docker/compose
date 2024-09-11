//go:build darwin
// +build darwin

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

package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	pathutil "github.com/docker/compose/v2/internal/paths"
	"github.com/fsnotify/fsevents"
	"github.com/sirupsen/logrus"
)

// A file watcher optimized for Darwin.
// Uses FSEvents to avoid the terrible perf characteristics of kqueue. Requires CGO
type fseventNotify struct {
	stream *fsevents.EventStream
	events chan FileEvent
	errors chan error
	stop   chan struct{}

	pathsWereWatching map[string]interface{}
	ignore            PathMatcher
}

func (d *fseventNotify) loop() {
	for {
		select {
		case <-d.stop:
			return
		case events, ok := <-d.stream.Events:
			if !ok {
				return
			}

			for _, e := range events {
				e.Path = filepath.Join(string(os.PathSeparator), e.Path)

				_, isPathWereWatching := d.pathsWereWatching[e.Path]
				if e.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir && e.Flags&fsevents.ItemCreated == fsevents.ItemCreated && isPathWereWatching {
					// This is the first create for the path that we're watching. We always get exactly one of these
					// even after we get the HistoryDone event. Skip it.
					continue
				}

				ignore, err := d.ignore.Matches(e.Path)
				if err != nil {
					logrus.Infof("Error matching path %q: %v", e.Path, err)
				} else if ignore {
					logrus.Tracef("Ignoring event for path: %v", e.Path)
					continue
				}

				d.events <- NewFileEvent(e.Path)
			}
		}
	}
}

// Add a path to be watched. Should only be called during initialization.
func (d *fseventNotify) initAdd(name string) {
	d.stream.Paths = append(d.stream.Paths, name)

	if d.pathsWereWatching == nil {
		d.pathsWereWatching = make(map[string]interface{})
	}
	d.pathsWereWatching[name] = struct{}{}
}

func (d *fseventNotify) Start() error {
	if len(d.stream.Paths) == 0 {
		return nil
	}

	numberOfWatches.Add(int64(len(d.stream.Paths)))

	d.stream.Start() //nolint:errcheck // FIXME(thaJeztah): should this return an error?

	go d.loop()

	return nil
}

func (d *fseventNotify) Close() error {
	numberOfWatches.Add(int64(-len(d.stream.Paths)))

	d.stream.Stop()
	close(d.errors)
	close(d.stop)

	return nil
}

func (d *fseventNotify) Events() chan FileEvent {
	return d.events
}

func (d *fseventNotify) Errors() chan error {
	return d.errors
}

func newWatcher(paths []string, ignore PathMatcher) (Notify, error) {
	dw := &fseventNotify{
		ignore: ignore,
		stream: &fsevents.EventStream{
			Latency: 50 * time.Millisecond,
			Flags:   fsevents.FileEvents | fsevents.IgnoreSelf,
			// NOTE(dmiller): this corresponds to the `sinceWhen` parameter in FSEventStreamCreate
			// https://developer.apple.com/documentation/coreservices/1443980-fseventstreamcreate
			EventID: fsevents.LatestEventID(),
		},
		events: make(chan FileEvent),
		errors: make(chan error),
		stop:   make(chan struct{}),
	}

	paths = pathutil.EncompassingPaths(paths)
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("newWatcher: %w", err)
		}
		dw.initAdd(path)
	}

	return dw, nil
}

var _ Notify = &fseventNotify{}
