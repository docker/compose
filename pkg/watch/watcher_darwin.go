//go:build fsnotify

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
	"sync"
	"time"

	"github.com/fsnotify/fsevents"
	"github.com/sirupsen/logrus"

	pathutil "github.com/docker/compose/v5/internal/paths"
)

// A file watcher optimized for Darwin.
// Uses FSEvents to avoid the terrible perf characteristics of kqueue. Requires CGO
type fseventNotify struct {
	stream *fsevents.EventStream
	events chan FileEvent
	errors chan error
	stop   chan struct{}

	// watchPaths are the paths registered with the FSEvents stream.
	watchPaths []string
	// notifyList holds the original trigger paths.
	notifyList map[string]bool
	// ignore maps each trigger path (from notifyList) to its own isolated PathMatcher
	ignore    map[string]PathMatcher
	closeOnce sync.Once
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

				isTriggerRoot := d.notifyList[e.Path]
				if e.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir && e.Flags&fsevents.ItemCreated == fsevents.ItemCreated && isTriggerRoot {
					// This is the first create for the path that we're watching. We always get exactly one of these
					// even after we get the HistoryDone event. Skip it.
					continue
				}

				if !d.shouldNotify(e.Path) {
					continue
				}

				d.events <- NewFileEvent(e.Path)
			}
		}
	}
}

// addStreamPath registers an existing ancestor path with the FSEvents stream.
func (d *fseventNotify) addStreamPath(name string) {
	d.stream.Paths = append(d.stream.Paths, name)
}

func (d *fseventNotify) Start() error {
	if len(d.watchPaths) == 0 {
		return nil
	}

	watchPaths, err := greatestExistingAncestors(d.watchPaths)
	if err != nil {
		return err
	}
	watchPaths = pathutil.EncompassingPaths(watchPaths)

	d.stream.Paths = nil
	for _, path := range watchPaths {
		d.addStreamPath(path)
	}

	d.closeOnce = sync.Once{}

	numberOfWatches.Add(int64(len(d.stream.Paths)))

	err = d.stream.Start()
	if err != nil {
		return err
	}
	go d.loop()
	return nil
}

func (d *fseventNotify) Close() error {
	d.closeOnce.Do(func() {
		numberOfWatches.Add(int64(-len(d.stream.Paths)))

		d.stream.Stop()
		close(d.errors)
		close(d.stop)
	})

	return nil
}

func (d *fseventNotify) Events() chan FileEvent {
	return d.events
}

func (d *fseventNotify) Errors() chan error {
	return d.errors
}

func (d *fseventNotify) shouldNotify(path string) bool {
	if d.notifyList[path] {
		stat, err := os.Lstat(path)
		isDir := err == nil && stat.IsDir()
		return !isDir
	}

	for root := range d.notifyList {
		if pathutil.IsChild(root, path) {
			if !d.shouldIgnore(root, path) {
				return true
			}
		}
	}
	return false
}

func (d *fseventNotify) shouldIgnore(dir, path string) bool {
	if len(d.ignore) == 0 {
		return false
	}

	if ignore, exists := d.ignore[dir]; exists && ignore != nil {
		matches, err := ignore.Matches(path)
		if err != nil {
			logrus.Debugf("error checking ignored path %q: %v", path, err)
			return false
		}
		return matches
	}
	return false
}

func newWatcher(paths []string, ignore map[string]PathMatcher) (Notify, error) {
	dw := &fseventNotify{
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

	watchPaths := pathutil.EncompassingPaths(paths)

	notifyList := make(map[string]bool, len(paths))
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("newWatcher: %w", err)
		}
		notifyList[abs] = true
	}

	dw.ignore = ignore
	dw.notifyList = notifyList
	dw.watchPaths = watchPaths

	return dw, nil
}

var _ Notify = &fseventNotify{}
