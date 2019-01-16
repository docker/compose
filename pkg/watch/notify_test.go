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

	"github.com/windmilleng/tilt/internal/testutils/tempdir"
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
		dir := f.TempDir("watched")
		dirs[i] = dir
		err := f.notify.Add(dir)
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

	root := f.TempDir("root")

	// add a sub directory
	subPath := filepath.Join(root, "sub")
	f.MkdirAll(subPath)

	// watch parent
	err := f.notify.Add(root)
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

	root := f.TempDir("root")

	// watch parent
	err := f.notify.Add(root)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()
	f.events = nil

	// add a sub directory
	subPath := filepath.Join(root, "sub")
	f.MkdirAll(subPath)

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

	root := f.TempDir("root")
	path := filepath.Join(root, "change")

	err := f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}

	f.fsync()

	d1 := "hello\ngo\n"
	f.WriteFile(path, d1)
	f.assertEvents(path)
}

func TestRemove(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	path := filepath.Join(root, "change")

	d1 := "hello\ngo\n"
	f.WriteFile(path, d1)

	err := f.notify.Add(path)
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

	path := filepath.Join(f.watched, "change")

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

	root := f.TempDir("root")
	path := filepath.Join(root, "change")

	d1 := "hello\ngo\n"
	f.WriteFile(path, d1)

	err := f.notify.Add(path)
	if err != nil {
		t.Fatal(err)
	}
	f.fsync()

	d2 := []byte("hello\nworld\n")
	err = ioutil.WriteFile(path, d2, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.assertEvents(path)
}

func TestWriteBrokenLink(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	link := filepath.Join(f.watched, "brokenLink")
	missingFile := filepath.Join(f.watched, "missingFile")
	err := os.Symlink(missingFile, link)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(link)
}

func TestWriteGoodLink(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	goodFile := filepath.Join(f.watched, "goodFile")
	err := ioutil.WriteFile(goodFile, []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(f.watched, "goodFileSymlink")
	err = os.Symlink(goodFile, link)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(goodFile, link)
}

func TestWatchBrokenLink(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	newRoot, err := NewDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer newRoot.TearDown()

	link := filepath.Join(newRoot.Path(), "brokenLink")
	missingFile := filepath.Join(newRoot.Path(), "missingFile")
	err = os.Symlink(missingFile, link)
	if err != nil {
		t.Fatal(err)
	}

	err = f.notify.Add(newRoot.Path())
	if err != nil {
		t.Fatal(err)
	}

	os.Remove(link)
	f.assertEvents(link)
}

func TestMoveAndReplace(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	file := filepath.Join(root, "myfile")
	f.WriteFile(file, "hello")

	err := f.notify.Add(file)
	if err != nil {
		t.Fatal(err)
	}

	tmpFile := filepath.Join(root, ".myfile.swp")
	f.WriteFile(tmpFile, "world")

	err = os.Rename(tmpFile, file)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(file)
}

func TestWatchBothDirAndFile(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	dir := f.JoinPath("foo")
	fileA := f.JoinPath("foo", "a")
	fileB := f.JoinPath("foo", "b")
	f.WriteFile(fileA, "a")
	f.WriteFile(fileB, "b")

	f.watch(fileA)
	f.watch(dir)
	f.fsync()
	f.events = nil

	f.WriteFile(fileB, "b-new")
	f.assertEvents(fileB)
}

func TestWatchNonexistentDirectory(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.JoinPath("root")
	err := os.Mkdir(root, 0777)
	if err != nil {
		t.Fatal(err)
	}
	parent := f.JoinPath("root", "parent")
	file := f.JoinPath("root", "parent", "a")

	f.watch(file)
	f.fsync()
	f.events = nil
	f.WriteFile(file, "hello")
	if runtime.GOOS == "darwin" {
		f.assertEvents(file)
	} else {
		f.assertEvents(parent, file)
	}
}

type notifyFixture struct {
	*tempdir.TempDirFixture
	notify  Notify
	watched string
	events  []FileEvent
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	SetLimitChecksEnabled(false)
	notify, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}

	f := tempdir.NewTempDirFixture(t)
	watched := f.TempDir("watched")

	err = notify.Add(watched)
	if err != nil {
		t.Fatal(err)
	}
	return &notifyFixture{
		TempDirFixture: f,
		watched:        watched,
		notify:         notify,
	}
}

func (f *notifyFixture) watch(path string) {
	err := f.notify.Add(path)
	if err != nil {
		f.T().Fatalf("notify.Add: %s", path)
	}
}

func (f *notifyFixture) assertEvents(expected ...string) {
	f.fsync()

	if len(f.events) != len(expected) {
		f.T().Fatalf("Got %d events (expected %d): %v %v", len(f.events), len(expected), f.events, expected)
	}

	for i, actual := range f.events {
		e := FileEvent{expected[i]}
		if actual != e {
			f.T().Fatalf("Got event %v (expected %v)", actual, e)
		}
	}
}

func (f *notifyFixture) fsync() {
	syncPathBase := fmt.Sprintf("sync-%d.txt", time.Now().UnixNano())
	syncPath := filepath.Join(f.watched, syncPathBase)
	anySyncPath := filepath.Join(f.watched, "sync-")
	timeout := time.After(time.Second)

	f.WriteFile(syncPath, fmt.Sprintf("%s", time.Now()))

F:
	for {
		select {
		case err := <-f.notify.Errors():
			f.T().Fatal(err)

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
			f.T().Fatalf("fsync: timeout")
		}
	}
}

func (f *notifyFixture) tearDown() {
	SetLimitChecksEnabled(true)

	err := f.notify.Close()
	if err != nil {
		f.T().Fatal(err)
	}

	f.TempDirFixture.TearDown()
}
