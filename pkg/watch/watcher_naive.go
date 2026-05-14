//go:build !fsnotify

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
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tilt-dev/fsnotify"

	pathutil "github.com/docker/compose/v5/internal/paths"
)

// A naive file watcher that uses the plain fsnotify API.
// Used on all non-Darwin systems (including Windows & Linux).
//
// All OS-specific codepaths are handled by fsnotify.
type naiveNotify struct {
	// Paths that we're watching that should be passed up to the caller.
	// Note that we may have to watch ancestors of these paths
	// in order to fulfill the API promise.
	//
	// We often need to check if paths are a child of a path in
	// the notify list. It might be better to store this in a tree
	// structure, so we can filter the list quickly.
	notifyList map[string]bool
	// ignore maps each notifyList root to the merged PathMatcher for paths under it.
	ignore     map[string]PathMatcher

	isWatcherRecursive bool
	watcher            *fsnotify.Watcher
	events             chan fsnotify.Event
	wrappedEvents      chan FileEvent
	errors             chan error
	numWatches         int64
}

func (d *naiveNotify) Start() error {
	if len(d.notifyList) == 0 {
		return nil
	}

	notifyRoots := make([]string, 0, len(d.notifyList))

	for path := range d.notifyList {
		notifyRoots = append(notifyRoots, path)
	}

	pathsToWatch, err := greatestExistingAncestors(notifyRoots, d.ignore)
	if err != nil {
		return err
	}
	if d.isWatcherRecursive {
		pathsToWatch = pathutil.EncompassingPaths(pathsToWatch)
	}

	_, d.ignore, err = normalizeWatchRoots(notifyRoots, d.ignore)
	if err != nil {
		return err
	}

	for _, name := range pathsToWatch {
		fi, err := os.Stat(name)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("notify.Add(%q): %w", name, err)
		}

		// if it's a file that doesn't exist,
		// we should have caught that above, let's just skip it.
		if os.IsNotExist(err) {
			continue
		}

		if fi.IsDir() {
			err = d.watchRecursively(name)
			if err != nil {
				return fmt.Errorf("notify.Add(%q): %w", name, err)
			}
		} else {
			err = d.add(filepath.Dir(name))
			if err != nil {
				return fmt.Errorf("notify.Add(%q): %w", filepath.Dir(name), err)
			}
		}
	}

	go d.loop()

	return nil
}

func (d *naiveNotify) watchRecursively(dir string) error {
	if d.isWatcherRecursive {
		err := d.add(dir)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("watcher.Add(%q): %w", dir, err)
	}

	return filepath.WalkDir(dir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return nil
		}

		if d.shouldSkipDir(path) {
			logrus.Debugf("Ignoring directory and its contents (recursively): %s", path)
			return filepath.SkipDir
		}

		err = d.add(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("watcher.Add(%q): %w", path, err)
		}
		return nil
	})
}

func (d *naiveNotify) Close() error {
	numberOfWatches.Add(-d.numWatches)
	d.numWatches = 0
	return d.watcher.Close()
}

func (d *naiveNotify) Events() chan FileEvent {
	return d.wrappedEvents
}

func (d *naiveNotify) Errors() chan error {
	return d.errors
}

func (d *naiveNotify) loop() { //nolint:gocyclo
	defer close(d.wrappedEvents)
	for e := range d.events {
		// The Windows fsnotify event stream sometimes gets events with empty names
		// that are also sent to the error stream. Hmmmm...
		if e.Name == "" {
			continue
		}

		if e.Op&fsnotify.Create != fsnotify.Create {
			if d.shouldNotify(e.Name) {
				d.wrappedEvents <- FileEvent(e.Name)
			}
			continue
		}

		if d.isWatcherRecursive {
			if d.shouldNotify(e.Name) {
				d.wrappedEvents <- FileEvent(e.Name)
			}
			continue
		}

		// If the watcher is not recursive, we have to walk the tree
		// and add watches manually. We fire the event while we're walking the tree.
		// because it's a bit more elegant that way.
		//
		// TODO(dbentley): if there's a delete should we call d.watcher.Remove to prevent leaking?
		err := filepath.WalkDir(e.Name, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.shouldNotify(path) {
				d.wrappedEvents <- FileEvent(path)
			}

			// TODO(dmiller): symlinks 😭

			shouldWatch := false
			if info.IsDir() {
				// watch directories unless we can skip them entirely
				if d.shouldSkipDir(path) {
					return filepath.SkipDir
				}

				shouldWatch = true
			} else {
				// watch files that are explicitly named, but don't watch others
				_, ok := d.notifyList[path]
				if ok {
					shouldWatch = true
				}
			}
			if shouldWatch {
				err := d.add(path)
				if err != nil && !os.IsNotExist(err) {
					logrus.Infof("Error watching path %s: %s", e.Name, err)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			logrus.Infof("Error walking directory %s: %s", e.Name, err)
		}
	}
}

func (d *naiveNotify) shouldNotify(path string) bool {
	if _, ok := d.notifyList[path]; ok {
		// We generally don't care when directories change at the root of an ADD
		stat, err := os.Lstat(path)
		isDir := err == nil && stat.IsDir()
		return !isDir
	}

	for root := range d.notifyList {
		if pathutil.IsChild(root, path) {
			return !d.shouldIgnore(root, path)
		}
	}
	return false
}

func (d *naiveNotify) shouldSkipDir(path string) bool {
	// If path is directly in the notifyList, we should always watch it.
	if d.notifyList[path] {
		return false
	}

	// Only walk directories under a notifyList path or under an ancestor of one
	// (Start() may watch a parent when the target is missing or is a file).
	// Decide ancestor/descendant versus notifyList before applying ignores so one
	// root's patterns cannot block reaching another root.
	// When walking beneath a watched ancestor, prune subtrees only with that root's
	// matcher from d.ignore.
	isChildOfWatchedDir := false
	var dir string
	for root := range d.notifyList {
		if pathutil.IsChild(path, root) {
			return false
		}
		if pathutil.IsChild(root, path) {
			isChildOfWatchedDir = true
			dir = root
		}
	}

	if isChildOfWatchedDir && d.shouldIgnoreEntireDir(dir, path) {
		return true
	}

	return !isChildOfWatchedDir
}

func (d *naiveNotify) shouldIgnoreEntireDir(dir, path string) bool {
	if len(d.ignore) == 0 {
		return false
	}

	if ignore, exists := d.ignore[dir]; exists && ignore != nil {
		matches, err := ignore.MatchesEntireDir(path)
		if err != nil {
			logrus.Debugf("error checking ignored directory %q: %v", path, err)
			return false
		}
		if matches {
			return true
		}
		return matches
	}
	return false
}

func (d *naiveNotify) shouldIgnore(dir, path string) bool {
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

func (d *naiveNotify) add(path string) error {
	err := d.watcher.Add(path)
	if err != nil {
		return err
	}
	d.numWatches++
	numberOfWatches.Add(1)
	return nil
}

func newWatcher(paths []string, ignore map[string]PathMatcher) (Notify, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		if strings.Contains(err.Error(), "too many open files") && runtime.GOOS == "linux" {
			return nil, fmt.Errorf("hit OS limits creating a watcher.\n" +
				"Run 'sysctl fs.inotify.max_user_instances' to check your inotify limits.\n" +
				"To raise them, run 'sudo sysctl fs.inotify.max_user_instances=1024'")
		}
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}
	MaybeIncreaseBufferSize(fsw)

	err = fsw.SetRecursive()
	isWatcherRecursive := err == nil

	wrappedEvents := make(chan FileEvent)

	watchRoots := paths
	if isWatcherRecursive {
		watchRoots = pathutil.EncompassingPaths(paths)
	}

	notifyList, normalizedIgnores, err := normalizeWatchRoots(watchRoots, ignore)
	if err != nil {
		return nil, fmt.Errorf("newWatcher: %w", err)
	}

	wmw := &naiveNotify{
		notifyList:         notifyList,
		ignore:             normalizedIgnores,
		watcher:            fsw,
		events:             fsw.Events,
		wrappedEvents:      wrappedEvents,
		errors:             fsw.Errors,
		isWatcherRecursive: isWatcherRecursive,
	}

	return wmw, nil
}

var _ Notify = &naiveNotify{}
