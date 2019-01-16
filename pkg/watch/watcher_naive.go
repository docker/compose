// +build !darwin

package watch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/windmilleng/fsnotify"
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
	watchList     map[string]bool
}

func (d *naiveNotify) Add(name string) error {
	fi, err := os.Stat(name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "notify.Add(%q)", name)
	}

	// if it's a file that doesn't exist, watch its parent
	if os.IsNotExist(err) {
		err, fileWatched := d.watchUpRecursively(name)
		if err != nil {
			return errors.Wrapf(err, "watchUpRecursively(%q)", name)
		}
		d.watchList[fileWatched] = true
	} else if fi.IsDir() {
		err = d.watchRecursively(name)
		if err != nil {
			return errors.Wrapf(err, "notify.Add(%q)", name)
		}
		d.watchList[name] = true
	} else {
		err = d.watcher.Add(name)
		if err != nil {
			return errors.Wrapf(err, "notify.Add(%q)", name)
		}
		d.watchList[name] = true
	}

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

func (d *naiveNotify) watchUpRecursively(path string) (error, string) {
	if path == string(filepath.Separator) {
		return fmt.Errorf("cannot watch root directory"), ""
	}

	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "os.Stat(%q)", path), ""
	}

	if os.IsNotExist(err) {
		parent := filepath.Dir(path)
		return d.watchUpRecursively(parent)
	}

	return d.watcher.Add(path), path
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
				d.sendEventIfWatched(newE)
				// TODO(dmiller): symlinks 😭
				err = d.Add(path)
				if err != nil {
					log.Printf("Error watching path %s: %s", e.Name, err)
				}
				return nil
			})
			if err != nil {
				log.Printf("Error walking directory %s: %s", e.Name, err)
			}
		} else {
			d.sendEventIfWatched(e)
		}
	}
}

func (d *naiveNotify) sendEventIfWatched(e fsnotify.Event) {
	if _, ok := d.watchList[e.Name]; ok {
		d.wrappedEvents <- FileEvent{e.Name}
	} else {
		// TODO(dmiller): maybe use a prefix tree here?
		for path := range d.watchList {
			if pathIsChildOf(e.Name, path) {
				d.wrappedEvents <- FileEvent{e.Name}
				break
			}
		}
	}
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
		watchList:     map[string]bool{},
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
