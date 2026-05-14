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

	pathsWereWatching map[string]any
	// ignore maps each pathsWereWatching root to the merged PathMatcher for paths under it.
	ignore            map[string]PathMatcher
	closeOnce         sync.Once
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
	notifyRoots := make([]string, 0, len(d.pathsWereWatching))
	for path := range d.pathsWereWatching {
		notifyRoots = append(notifyRoots, path)
	}
	if len(notifyRoots) == 0 {
		return nil
	}

	pathsToWatch, err := greatestExistingAncestors(notifyRoots, d.ignore)
	if err != nil {
		return err
	}
	pathsToWatch = pathutil.EncompassingPaths(pathsToWatch)

	_, normalizedIgnores, err := normalizeWatchRoots(notifyRoots, d.ignore)
	if err != nil {
		return err
	}
	d.ignore = normalizedIgnores
	d.stream.Paths = nil
	for _, path := range pathsToWatch {
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

	if _, ok := d.pathsWereWatching[path]; ok {
		stat, err := os.Lstat(path)
		isDir := err == nil && stat.IsDir()
		return !isDir
	}

	for root := range d.pathsWereWatching {
		if pathutil.IsChild(root, path) {
			if d.shouldIgnore(root, path) {
				return false
			}
			return true
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

	watchRoots := pathutil.EncompassingPaths(paths)
	notifyList, normalizedIgnores, err := normalizeWatchRoots(watchRoots, ignore)
	if err != nil {
		return nil, fmt.Errorf("newWatcher: %w", err)
	}
	dw.ignore = normalizedIgnores
	dw.pathsWereWatching = make(map[string]any, len(notifyList))
	for path := range notifyList {
		dw.pathsWereWatching[path] = struct{}{}
	}

	return dw, nil
}

var _ Notify = &fseventNotify{}
