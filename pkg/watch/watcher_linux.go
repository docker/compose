package watch

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/windmilleng/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

const enospc = "no space left on device"
const inotifyErrMsg = "The user limit on the total number of inotify watches was reached; increase the fs.inotify.max_user_watches sysctl. See here for more information: https://facebook.github.io/watchman/docs/install.html#linux-inotify-limits"
const inotifyMin = 8192

type linuxNotify struct {
	watcher       *fsnotify.Watcher
	events        chan fsnotify.Event
	wrappedEvents chan FileEvent
	errors        chan error
	watchList     map[string]bool
}

func (d *linuxNotify) Add(name string) error {
	fi, err := os.Stat(name)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("notify.Add(%q): %v", name, err)
	}

	// if it's a file that doesn't exist watch it's parent
	if os.IsNotExist(err) {
		parent := filepath.Join(name, "..")
		err = d.watcher.Add(parent)
		if err != nil {
			return fmt.Errorf("notify.Add(%q): %v", name, err)
		}
		d.watchList[parent] = true
	} else if fi.IsDir() {
		err = d.watchRecursively(name)
		if err != nil {
			return fmt.Errorf("notify.Add(%q): %v", name, err)
		}
		d.watchList[name] = true
	} else {
		err = d.watcher.Add(name)
		if err != nil {
			return fmt.Errorf("notify.Add(%q): %v", name, err)
		}
		d.watchList[name] = true
	}

	return nil
}

func (d *linuxNotify) watchRecursively(dir string) error {
	return filepath.Walk(dir, func(path string, mode os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		err = d.watcher.Add(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("watcher.Add(%q): %v", path, err)
		}
		return nil
	})
}

func (d *linuxNotify) Close() error {
	return d.watcher.Close()
}

func (d *linuxNotify) Events() chan FileEvent {
	return d.wrappedEvents
}

func (d *linuxNotify) Errors() chan error {
	return d.errors
}

func (d *linuxNotify) loop() {
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

func (d *linuxNotify) sendEventIfWatched(e fsnotify.Event) {
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

func NewWatcher() (*linuxNotify, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	wrappedEvents := make(chan FileEvent)

	wmw := &linuxNotify{
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

func checkInotifyLimits() error {
	if !LimitChecksEnabled() {
		return nil
	}

	data, err := ioutil.ReadFile("/proc/sys/fs/inotify/max_user_watches")
	if err != nil {
		return err
	}

	i, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}

	if i < inotifyMin {
		return grpc.Errorf(
			codes.ResourceExhausted,
			"The user limit on the total number of inotify watches is too low (%d); increase the fs.inotify.max_user_watches sysctl. See here for more information: https://facebook.github.io/watchman/docs/install.html#linux-inotify-limits",
			i,
		)
	}

	return nil
}

var _ Notify = &linuxNotify{}
