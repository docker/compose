// +build !darwin

package watch

import (
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/windmilleng/fsnotify"

	"github.com/windmilleng/tilt/internal/logger"
	"github.com/windmilleng/tilt/internal/ospath"
)

// A naive file watcher that uses the plain fsnotify API.
// Used on all non-Darwin systems (including Windows & Linux).
//
// All OS-specific codepaths are handled by fsnotify.
type naiveNotify struct {
	log           logger.Logger
	watcher       *fsnotify.Watcher
	events        chan fsnotify.Event
	wrappedEvents chan FileEvent
	errors        chan error

	mu sync.Mutex

	// Paths that we're watching that should be passed up to the caller.
	// Note that we may have to watch ancestors of these paths
	// in order to fulfill the API promise.
	notifyList map[string]bool
}

var (
	numberOfWatches = expvar.NewInt("watch.naive.numberOfWatches")
)

func (d *naiveNotify) Add(name string) error {
	fi, err := os.Stat(name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "notify.Add(%q)", name)
	}

	// if it's a file that doesn't exist, watch its parent
	if os.IsNotExist(err) {
		err = d.watchAncestorOfMissingPath(name)
		if err != nil {
			return errors.Wrapf(err, "watchAncestorOfMissingPath(%q)", name)
		}
	} else if fi.IsDir() {
		err = d.watchRecursively(name)
		if err != nil {
			return errors.Wrapf(err, "notify.Add(%q)", name)
		}
	} else {
		err = d.add(filepath.Dir(name))
		if err != nil {
			return errors.Wrapf(err, "notify.Add(%q)", filepath.Dir(name))
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.notifyList[name] = true

	return nil
}

func (d *naiveNotify) watchRecursively(dir string) error {
	return filepath.Walk(dir, func(path string, mode os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !mode.IsDir() {
			return nil
		}
		err = d.add(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return errors.Wrapf(err, "watcher.Add(%q)", path)
		}
		return nil
	})
}

func (d *naiveNotify) watchAncestorOfMissingPath(path string) error {
	if path == string(filepath.Separator) {
		return fmt.Errorf("cannot watch root directory")
	}

	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "os.Stat(%q)", path)
	}

	if os.IsNotExist(err) {
		parent := filepath.Dir(path)
		return d.watchAncestorOfMissingPath(parent)
	}

	return d.add(path)
}

func (d *naiveNotify) Close() error {
	return d.watcher.Close()
}

func (d *naiveNotify) Events() chan FileEvent {
	return d.wrappedEvents
}

func (d *naiveNotify) Errors() chan error {
	return d.errors
}

func (d *naiveNotify) loop() {
	defer close(d.wrappedEvents)
	for e := range d.events {
		shouldNotify := d.shouldNotify(e.Name)

		if e.Op&fsnotify.Create != fsnotify.Create {
			if shouldNotify {
				d.wrappedEvents <- FileEvent{e.Name}
			}
			continue
		}

		// TODO(dbentley): if there's a delete should we call d.watcher.Remove to prevent leaking?
		err := filepath.Walk(e.Name, func(path string, mode os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if d.shouldNotify(path) {
				d.wrappedEvents <- FileEvent{path}
			}

			// TODO(dmiller): symlinks 😭

			shouldWatch := false
			if mode.IsDir() {
				// watch all directories
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
					d.log.Infof("Error watching path %s: %s", e.Name, err)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			d.log.Infof("Error walking directory %s: %s", e.Name, err)
		}
	}
}

func (d *naiveNotify) shouldNotify(path string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.notifyList[path]; ok {
		return true
	}
	// TODO(dmiller): maybe use a prefix tree here?
	for root := range d.notifyList {
		if ospath.IsChild(root, path) {
			return true
		}
	}
	return false
}

func (d *naiveNotify) add(path string) error {
	err := d.watcher.Add(path)
	if err != nil {
		return err
	}
	numberOfWatches.Add(1)
	return nil
}

func NewWatcher(l logger.Logger) (*naiveNotify, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	wrappedEvents := make(chan FileEvent)

	wmw := &naiveNotify{
		log:           l,
		watcher:       fsw,
		events:        fsw.Events,
		wrappedEvents: wrappedEvents,
		errors:        fsw.Errors,
		notifyList:    map[string]bool{},
	}

	go wmw.loop()

	return wmw, nil
}

func isDir(pth string) (bool, error) {
	fi, err := os.Lstat(pth)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

var _ Notify = &naiveNotify{}
