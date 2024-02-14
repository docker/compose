//go:build !darwin
// +build !darwin

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

	pathutil "github.com/docker/compose/v2/internal/paths"
	"github.com/sirupsen/logrus"
	"github.com/tilt-dev/fsnotify"
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

	ignore PathMatcher

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

	pathsToWatch := []string{}
	for path := range d.notifyList {
		pathsToWatch = append(pathsToWatch, path)
	}

	pathsToWatch, err := greatestExistingAncestors(pathsToWatch)
	if err != nil {
		return err
	}
	if d.isWatcherRecursive {
		pathsToWatch = pathutil.EncompassingPaths(pathsToWatch)
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

		shouldSkipDir, err := d.shouldSkipDir(path)
		if err != nil {
			return err
		}

		if shouldSkipDir {
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
				d.wrappedEvents <- FileEvent{e.Name}
			}
			continue
		}

		if d.isWatcherRecursive {
			if d.shouldNotify(e.Name) {
				d.wrappedEvents <- FileEvent{e.Name}
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
				d.wrappedEvents <- FileEvent{path}
			}

			// TODO(dmiller): symlinks 😭

			shouldWatch := false
			if info.IsDir() {
				// watch directories unless we can skip them entirely
				shouldSkipDir, err := d.shouldSkipDir(path)
				if err != nil {
					return err
				}
				if shouldSkipDir {
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
	ignore, err := d.ignore.Matches(path)
	if err != nil {
		logrus.Infof("Error matching path %q: %v", path, err)
	} else if ignore {
		logrus.Tracef("Ignoring event for path: %v", path)
		return false
	}

	if _, ok := d.notifyList[path]; ok {
		// We generally don't care when directories change at the root of an ADD
		stat, err := os.Lstat(path)
		isDir := err == nil && stat.IsDir()
		return !isDir
	}

	for root := range d.notifyList {
		if pathutil.IsChild(root, path) {
			return true
		}
	}
	return false
}

func (d *naiveNotify) shouldSkipDir(path string) (bool, error) {
	// If path is directly in the notifyList, we should always watch it.
	if d.notifyList[path] {
		return false, nil
	}

	skip, err := d.ignore.MatchesEntireDir(path)
	if err != nil {
		return false, fmt.Errorf("shouldSkipDir: %w", err)
	}

	if skip {
		return true, nil
	}

	// Suppose we're watching
	// /src/.tiltignore
	// but the .tiltignore file doesn't exist.
	//
	// Our watcher will create an inotify watch on /src/.
	//
	// But then we want to make sure we don't recurse from /src/ down to /src/node_modules.
	//
	// To handle this case, we only want to traverse dirs that are:
	// - A child of a directory that's in our notify list, or
	// - A parent of a directory that's in our notify list
	//   (i.e., to cover the "path doesn't exist" case).
	for root := range d.notifyList {
		if pathutil.IsChild(root, path) || pathutil.IsChild(path, root) {
			return false, nil
		}
	}
	return true, nil
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

func newWatcher(paths []string, ignore PathMatcher) (Notify, error) {
	if ignore == nil {
		return nil, fmt.Errorf("newWatcher: ignore is nil")
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		if strings.Contains(err.Error(), "too many open files") && runtime.GOOS == "linux" {
			return nil, fmt.Errorf("Hit OS limits creating a watcher.\n" +
				"Run 'sysctl fs.inotify.max_user_instances' to check your inotify limits.\n" +
				"To raise them, run 'sudo sysctl fs.inotify.max_user_instances=1024'")
		}
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}
	MaybeIncreaseBufferSize(fsw)

	err = fsw.SetRecursive()
	isWatcherRecursive := err == nil

	wrappedEvents := make(chan FileEvent)
	notifyList := make(map[string]bool, len(paths))
	if isWatcherRecursive {
		paths = pathutil.EncompassingPaths(paths)
	}
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("newWatcher: %w", err)
		}
		notifyList[path] = true
	}

	wmw := &naiveNotify{
		notifyList:         notifyList,
		ignore:             ignore,
		watcher:            fsw,
		events:             fsw.Events,
		wrappedEvents:      wrappedEvents,
		errors:             fsw.Errors,
		isWatcherRecursive: isWatcherRecursive,
	}

	return wmw, nil
}

var _ Notify = &naiveNotify{}

func greatestExistingAncestors(paths []string) ([]string, error) {
	result := []string{}
	for _, p := range paths {
		newP, err := greatestExistingAncestor(p)
		if err != nil {
			return nil, fmt.Errorf("Finding ancestor of %s: %w", p, err)
		}
		result = append(result, newP)
	}
	return result, nil
}
