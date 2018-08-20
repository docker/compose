package watch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/windmilleng/fsnotify"
)

// Each implementation of the notify interface should have the same basic
// behavior.

func TestNoEvents(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()
	f.fsync()
	f.assertEvents()
}

func TestEventOrdering(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	count := 8
	dirs := make([]string, count)
	for i, _ := range dirs {
		dir, err := f.root.NewDir("watched")
		if err != nil {
			t.Fatal(err)
		}
		dirs[i] = dir.Path()
		err = f.notify.Add(dir.Path())
		if err != nil {
			t.Fatal(err)
		}
	}

	f.fsync()
	f.events = nil

	var expected []fsnotify.Event
	for i, dir := range dirs {
		base := fmt.Sprintf("%d.txt", i)
		p := filepath.Join(dir, base)
		err := ioutil.WriteFile(p, []byte(base), os.FileMode(0777))
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, create(filepath.Join(dir, base)))
	}

	f.fsync()

	f.filterJustCreateEvents()
	f.assertEvents(expected...)

	// Check to make sure that the files appeared in the right order.
	createEvents := make([]fsnotify.Event, 0, count)
	for _, e := range f.events {
		if e.Op == fsnotify.Create {
			createEvents = append(createEvents, e)
		}
	}

	if len(createEvents) != count {
		t.Fatalf("Expected %d create events. Actual: %+v", count, createEvents)
	}

	for i, event := range createEvents {
		base := fmt.Sprintf("%d.txt", i)
		p := filepath.Join(dirs[i], base)
		if event.Name != p {
			t.Fatalf("Expected event %q at %d. Actual: %+v", base, i, createEvents)
		}
	}
}

func TestWatchesAreRecursive(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	// add a sub directory
	subPath := filepath.Join(root.Path(), "sub")
	err = os.MkdirAll(subPath, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	// watch parent
	err = f.notify.Add(root.Path())
	if err != nil {
		t.Fatal(err)
	}

	f.fsync()
	f.events = nil
	// change sub directory
	changeFilePath := filepath.Join(subPath, "change")
	_, err = os.OpenFile(changeFilePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatal(err)
	}

	// we should get notified
	f.fsync()

	f.assertEvents(create(changeFilePath))
}

func TestNewDirectoriesAreRecursivelyWatched(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	// watch parent
	err = f.notify.Add(root.Path())
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()
	f.events = nil
	// add a sub directory
	subPath := filepath.Join(root.Path(), "sub")
	err = os.MkdirAll(subPath, os.ModePerm)
	if err != nil {
		f.t.Fatal(err)
	}
	// change something inside sub directory
	changeFilePath := filepath.Join(subPath, "change")
	_, err = os.OpenFile(changeFilePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatal(err)
	}
	// we should get notified
	f.fsync()
	// assert events
	f.assertEvents(create(subPath), create(changeFilePath))
}

func TestWatchNonExistentPath(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root.Path(), "change")

	err = f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	d1 := []byte("hello\ngo\n")
	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()
	if runtime.GOOS == "darwin" {
		f.assertEvents(create(path))
	} else {
		f.assertEvents(create(path), write(path))
	}
}

func TestRemove(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root.Path(), "change")

	if err != nil {
		t.Fatal(err)
	}
	d1 := []byte("hello\ngo\n")
	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()
	f.events = nil
	err = os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()

	f.assertEvents(remove(path))
}

func TestRemoveAndAddBack(t *testing.T) {
	t.Skip("Skipping broken test for now")
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root.Path(), "change")

	if err != nil {
		t.Fatal(err)
	}
	d1 := []byte("hello\ngo\n")
	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()

	f.assertEvents(remove(path))
	f.events = nil

	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(create(path))
}

func TestSingleFile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Broken on Linux")
	}
	f := newNotifyFixture(t)
	defer f.tearDown()

	root, err := f.root.NewDir("root")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root.Path(), "change")

	if err != nil {
		t.Fatal(err)
	}
	d1 := []byte("hello\ngo\n")
	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}

	d2 := []byte("hello\nworld\n")
	err = ioutil.WriteFile(path, d2, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()

	f.assertEvents(create(path))
}

type notifyFixture struct {
	t       *testing.T
	root    *TempDir
	watched *TempDir
	notify  Notify
	events  []fsnotify.Event
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	SetLimitChecksEnabled(false)
	notify, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}

	root, err := NewDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	watched, err := root.NewDir("watched")
	if err != nil {
		t.Fatal(err)
	}

	err = notify.Add(watched.Path())
	if err != nil {
		t.Fatal(err)
	}
	return &notifyFixture{
		t:       t,
		root:    root,
		watched: watched,
		notify:  notify,
	}
}

func (f *notifyFixture) filterJustCreateEvents() {
	var r []fsnotify.Event

	for _, ev := range f.events {
		if ev.Op != fsnotify.Create {
			continue
		}
		r = append(r, ev)
	}

	f.events = r
}

func (f *notifyFixture) assertEvents(expected ...fsnotify.Event) {
	if len(f.events) != len(expected) {
		f.t.Fatalf("Got %d events (expected %d): %v %v", len(f.events), len(expected), f.events, expected)
	}

	for i, actual := range f.events {
		if actual != expected[i] {
			f.t.Fatalf("Got event %v (expected %v)", actual, expected[i])
		}
	}
}

func create(f string) fsnotify.Event {
	return fsnotify.Event{
		Name: f,
		Op:   fsnotify.Create,
	}
}

func write(f string) fsnotify.Event {
	return fsnotify.Event{
		Name: f,
		Op:   fsnotify.Write,
	}
}

func remove(f string) fsnotify.Event {
	return fsnotify.Event{
		Name: f,
		Op:   fsnotify.Remove,
	}
}

func (f *notifyFixture) fsync() {
	syncPathBase := fmt.Sprintf("sync-%d.txt", time.Now().UnixNano())
	syncPath := filepath.Join(f.watched.Path(), syncPathBase)
	anySyncPath := filepath.Join(f.watched.Path(), "sync-")
	timeout := time.After(time.Second)

	err := ioutil.WriteFile(syncPath, []byte(fmt.Sprintf("%s", time.Now())), os.FileMode(0777))
	if err != nil {
		f.t.Fatal(err)
	}

F:
	for {
		select {
		case err := <-f.notify.Errors():
			f.t.Fatal(err)

		case event := <-f.notify.Events():
			if strings.Contains(event.Name, syncPath) {
				break F
			}
			if strings.Contains(event.Name, anySyncPath) {
				continue
			}
			f.events = append(f.events, event)

		case <-timeout:
			f.t.Fatalf("fsync: timeout")
		}
	}

	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *notifyFixture) tearDown() {
	SetLimitChecksEnabled(true)
	err := f.root.TearDown()
	if err != nil {
		f.t.Fatal(err)
	}

	err = f.notify.Close()
	if err != nil {
		f.t.Fatal(err)
	}
}
