// +build !darwin

package watch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/windmilleng/fsnotify"

	"github.com/windmilleng/tilt/internal/ospath"
)

// A naive file watcher that uses the plain fsnotify API.
// Used on all non-Darwin systems (including Windows & Linux).
//
// All OS-specific codepaths are handled by fsnotify.
type naiveNotify struct {
	watcher       *fsnotify.Watcher
	events        chan fsnotify.Event
	wrappedEvents chan FileEvent
	errors        chan error

	// Paths that we're watching that should be passed up to the caller.
	// Note that we may have to watch ancestors of these paths
	// in order to fulfill the API promise.
	notifyList map[string]bool
}

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
		err = d.watcher.Add(name)
		if err != nil {
			return errors.Wrapf(err, "notify.Add(%q)", name)
		}
	}
	d.notifyList[name] = true

	return nil
}

func (d *naiveNotify) watchRecursively(dir string) error {
	return filepath.Walk(dir, func(path string, mode os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		err = d.watcher.Add(path)
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

	return d.watcher.Add(path)
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
	for e := range d.events {
		isCreateOp := e.Op&fsnotify.Create == fsnotify.Create
		shouldWalk := false
		if isCreateOp {
			isDir, err := isDir(e.Name)
			if err != nil {
				log.Printf("Error stat-ing file %s: %s", e.Name, err)
				continue
			}
			shouldWalk = isDir
		}
		if shouldWalk {
			err := filepath.Walk(e.Name, func(path string, mode os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				newE := fsnotify.Event{
					Op:   fsnotify.Create,
					Name: path,
				}

				if d.shouldNotify(newE) {
					d.wrappedEvents <- FileEvent{newE.Name}

					// TODO(dmiller): symlinks 😭
					err = d.Add(path)
					if err != nil {
						log.Printf("Error watching path %s: %s", e.Name, err)
					}
				}
				return nil
			})
			if err != nil {
				log.Printf("Error walking directory %s: %s", e.Name, err)
			}
		} else if d.shouldNotify(e) {
			d.wrappedEvents <- FileEvent{e.Name}
		}
	}
}

func (d *naiveNotify) shouldNotify(e fsnotify.Event) bool {
	if _, ok := d.notifyList[e.Name]; ok {
		return true
	} else {
		// TODO(dmiller): maybe use a prefix tree here?
		for path := range d.notifyList {
			if ospath.IsChild(path, e.Name) {
				return true
			}
		}
	}
	return false
}

func NewWatcher() (*naiveNotify, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	wrappedEvents := make(chan FileEvent)

	wmw := &naiveNotify{
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
