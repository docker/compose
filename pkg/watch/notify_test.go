package watch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Each implementation of the notify interface should have the same basic
// behavior.

func TestNoEvents(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()
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

	var expected []string
	for i, dir := range dirs {
		base := fmt.Sprintf("%d.txt", i)
		p := filepath.Join(dir, base)
		err := ioutil.WriteFile(p, []byte(base), os.FileMode(0777))
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, filepath.Join(dir, base))
	}

	f.assertEvents(expected...)
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

	f.assertEvents(changeFilePath)
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
	f.assertEvents(subPath, changeFilePath)
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
	f.assertEvents(path)
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
	f.assertEvents(path)
}

func TestRemoveAndAddBack(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	path := filepath.Join(f.watched.Path(), "change")

	d1 := []byte("hello\ngo\n")
	err := ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	f.assertEvents(path)

	err = os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(path)
	f.events = nil

	err = ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(path)
}

func TestSingleFile(t *testing.T) {
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
	f.assertEvents(path)
}

type notifyFixture struct {
	t       *testing.T
	root    *TempDir
	watched *TempDir
	notify  Notify
	events  []FileEvent
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

func (f *notifyFixture) assertEvents(expected ...string) {
	f.fsync()

	if len(f.events) != len(expected) {
		f.t.Fatalf("Got %d events (expected %d): %v %v", len(f.events), len(expected), f.events, expected)
	}

	for i, actual := range f.events {
		e := FileEvent{expected[i]}
		if actual != e {
			f.t.Fatalf("Got event %v (expected %v)", actual, e)
		}
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
			if strings.Contains(event.Path, syncPath) {
				break F
			}
			if strings.Contains(event.Path, anySyncPath) {
				continue
			}

			// Don't bother tracking duplicate changes to the same path
			// for testing.
			if len(f.events) > 0 && f.events[len(f.events)-1].Path == event.Path {
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
