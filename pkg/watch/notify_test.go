package watch

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/windmilleng/tilt/internal/dockerignore"
	"github.com/windmilleng/tilt/internal/testutils/tempdir"
	"github.com/windmilleng/tilt/pkg/logger"
)

// Each implementation of the notify interface should have the same basic
// behavior.

func TestNoEvents(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()
	f.assertEvents()
}

func TestNoWatches(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()
	f.paths = nil
	f.rebuildWatcher()
	f.assertEvents()
}

func TestEventOrdering(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	count := 8
	dirs := make([]string, count)
	for i := range dirs {
		dir := f.TempDir("watched")
		dirs[i] = dir
		f.watch(dir)
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

// Simulate a git branch switch that creates a bunch
// of directories, creates files in them, then deletes
// them all quickly. Make sure there are no errors.
func TestGitBranchSwitch(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	count := 10
	dirs := make([]string, count)
	for i := range dirs {
		dir := f.TempDir("watched")
		dirs[i] = dir
		f.watch(dir)
	}

	f.fsync()
	f.events = nil

	// consume all the events in the background
	ctx, cancel := context.WithCancel(context.Background())
	done := f.consumeEventsInBackground(ctx)

	for i, dir := range dirs {
		for j := 0; j < count; j++ {
			base := fmt.Sprintf("x/y/dir-%d/x.txt", j)
			p := filepath.Join(dir, base)
			f.WriteFile(p, "contents")
		}

		if i != 0 {
			err := os.RemoveAll(dir)
			require.NoError(t, err)
		}
	}

	cancel()
	err := <-done
	if err != nil {
		t.Fatal(err)
	}

	f.fsync()
	f.events = nil

	// Make sure the watch on the first dir still works.
	dir := dirs[0]
	path := filepath.Join(dir, "change")

	f.WriteFile(path, "hello\n")
	f.fsync()

	f.assertEvents(path)

	// Make sure there are no errors in the out stream
	assert.Equal(t, "", f.out.String())
}

func TestWatchesAreRecursive(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")

	// add a sub directory
	subPath := filepath.Join(root, "sub")
	f.MkdirAll(subPath)

	// watch parent
	f.watch(root)

	f.fsync()
	f.events = nil
	// change sub directory
	changeFilePath := filepath.Join(subPath, "change")
	_, err := os.OpenFile(changeFilePath, os.O_RDONLY|os.O_CREATE, 0666)
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
	f.watch(root)
	f.fsync()
	f.events = nil

	// add a sub directory
	subPath := filepath.Join(root, "sub")
	f.MkdirAll(subPath)

	// change something inside sub directory
	changeFilePath := filepath.Join(subPath, "change")
	_, err := os.OpenFile(changeFilePath, os.O_RDONLY|os.O_CREATE, 0666)
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

	f.watch(path)
	f.fsync()

	d1 := "hello\ngo\n"
	f.WriteFile(path, d1)
	f.assertEvents(path)
}

func TestWatchNonExistentPathDoesNotFireSiblingEvent(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	watchedFile := filepath.Join(root, "a.txt")
	unwatchedSibling := filepath.Join(root, "b.txt")

	f.watch(watchedFile)
	f.fsync()

	d1 := "hello\ngo\n"
	f.WriteFile(unwatchedSibling, d1)
	f.assertEvents()
}

func TestRemove(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	path := filepath.Join(root, "change")

	d1 := "hello\ngo\n"
	f.WriteFile(path, d1)

	f.watch(path)
	f.fsync()
	f.events = nil
	err := os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}
	f.assertEvents(path)
}

func TestRemoveAndAddBack(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	path := filepath.Join(f.paths[0], "change")

	d1 := []byte("hello\ngo\n")
	err := ioutil.WriteFile(path, d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.watch(path)
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

	f.watch(path)
	f.fsync()

	d2 := []byte("hello\nworld\n")
	err := ioutil.WriteFile(path, d2, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.assertEvents(path)
}

func TestWriteBrokenLink(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	link := filepath.Join(f.paths[0], "brokenLink")
	missingFile := filepath.Join(f.paths[0], "missingFile")
	err := os.Symlink(missingFile, link)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents(link)
}

func TestWriteGoodLink(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	goodFile := filepath.Join(f.paths[0], "goodFile")
	err := ioutil.WriteFile(goodFile, []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(f.paths[0], "goodFileSymlink")
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
	defer func() {
		err := newRoot.TearDown()
		if err != nil {
			fmt.Printf("error tearing down temp dir: %v\n", err)
		}
	}()

	link := filepath.Join(newRoot.Path(), "brokenLink")
	missingFile := filepath.Join(newRoot.Path(), "missingFile")
	err = os.Symlink(missingFile, link)
	if err != nil {
		t.Fatal(err)
	}

	f.watch(newRoot.Path())
	err = os.Remove(link)
	require.NoError(t, err)
	f.assertEvents(link)
}

func TestMoveAndReplace(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	file := filepath.Join(root, "myfile")
	f.WriteFile(file, "hello")

	f.watch(file)
	tmpFile := filepath.Join(root, ".myfile.swp")
	f.WriteFile(tmpFile, "world")

	err := os.Rename(tmpFile, file)
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

func TestWatchNonexistentFileInNonexistentDirectoryCreatedSimultaneously(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.JoinPath("root")
	err := os.Mkdir(root, 0777)
	if err != nil {
		t.Fatal(err)
	}
	file := f.JoinPath("root", "parent", "a")

	f.watch(file)
	f.fsync()
	f.events = nil
	f.WriteFile(file, "hello")
	f.assertEvents(file)
}

func TestWatchNonexistentDirectory(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.JoinPath("root")
	err := os.Mkdir(root, 0777)
	if err != nil {
		t.Fatal(err)
	}
	parent := f.JoinPath("parent")
	file := f.JoinPath("parent", "a")

	f.watch(parent)
	f.fsync()
	f.events = nil

	err = os.Mkdir(parent, 0777)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "darwin" {
		// for directories that were the root of an Add, we don't report creation, cf. watcher_darwin.go
		f.assertEvents()
	} else {
		f.assertEvents(parent)
	}
	f.WriteFile(file, "hello")

	if runtime.GOOS == "darwin" {
		// mac doesn't return the dir change as part of file creation
		f.assertEvents(file)
	} else {
		f.assertEvents(parent, file)
	}
}

func TestWatchNonexistentFileInNonexistentDirectory(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.JoinPath("root")
	err := os.Mkdir(root, 0777)
	if err != nil {
		t.Fatal(err)
	}
	parent := f.JoinPath("parent")
	file := f.JoinPath("parent", "a")

	f.watch(file)
	f.assertEvents()

	err = os.Mkdir(parent, 0777)
	if err != nil {
		t.Fatal(err)
	}

	f.assertEvents()
	f.WriteFile(file, "hello")
	f.assertEvents(file)
}

func TestWatchCountInnerFile(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.paths[0]
	a := f.JoinPath(root, "a")
	b := f.JoinPath(a, "b")
	file := f.JoinPath(b, "bigFile")
	f.WriteFile(file, "hello")
	f.assertEvents(a, b, file)

	expectedWatches := 3
	if runtime.GOOS == "darwin" {
		expectedWatches = 1
	}
	assert.Equal(t, expectedWatches, int(numberOfWatches.Value()))
}

func TestWatchCountInnerFileWithIgnore(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.paths[0]
	ignore, _ := dockerignore.NewDockerPatternMatcher(root, []string{
		"a",
		"!a/b",
	})
	f.setIgnore(ignore)

	a := f.JoinPath(root, "a")
	b := f.JoinPath(a, "b")
	file := f.JoinPath(b, "bigFile")
	f.WriteFile(file, "hello")
	f.assertEvents(b, file)

	expectedWatches := 3
	if runtime.GOOS == "darwin" {
		expectedWatches = 1
	}
	assert.Equal(t, expectedWatches, int(numberOfWatches.Value()))
}

func TestIgnoreCreatedDir(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.paths[0]
	ignore, _ := dockerignore.NewDockerPatternMatcher(root, []string{"a/b"})
	f.setIgnore(ignore)

	a := f.JoinPath(root, "a")
	b := f.JoinPath(a, "b")
	file := f.JoinPath(b, "bigFile")
	f.WriteFile(file, "hello")
	f.assertEvents(a)

	expectedWatches := 2
	if runtime.GOOS == "darwin" {
		expectedWatches = 1
	}
	assert.Equal(t, expectedWatches, int(numberOfWatches.Value()))
}

func TestIgnoreCreatedDirWithExclusions(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.paths[0]
	ignore, _ := dockerignore.NewDockerPatternMatcher(root,
		[]string{
			"a/b",
			"c",
			"!c/d",
		})
	f.setIgnore(ignore)

	a := f.JoinPath(root, "a")
	b := f.JoinPath(a, "b")
	file := f.JoinPath(b, "bigFile")
	f.WriteFile(file, "hello")
	f.assertEvents(a)

	expectedWatches := 2
	if runtime.GOOS == "darwin" {
		expectedWatches = 1
	}
	assert.Equal(t, expectedWatches, int(numberOfWatches.Value()))
}

func TestIgnoreInitialDir(t *testing.T) {
	f := newNotifyFixture(t)
	defer f.tearDown()

	root := f.TempDir("root")
	ignore, _ := dockerignore.NewDockerPatternMatcher(root, []string{"a/b"})
	f.setIgnore(ignore)

	a := f.JoinPath(root, "a")
	b := f.JoinPath(a, "b")
	file := f.JoinPath(b, "bigFile")
	f.WriteFile(file, "hello")
	f.watch(root)

	f.assertEvents()

	expectedWatches := 3
	if runtime.GOOS == "darwin" {
		expectedWatches = 2
	}
	assert.Equal(t, expectedWatches, int(numberOfWatches.Value()))
}

type notifyFixture struct {
	out *bytes.Buffer
	*tempdir.TempDirFixture
	notify Notify
	ignore PathMatcher
	paths  []string
	events []FileEvent
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	out := bytes.NewBuffer(nil)
	nf := &notifyFixture{
		TempDirFixture: tempdir.NewTempDirFixture(t),
		paths:          []string{},
		ignore:         EmptyMatcher{},
		out:            out,
	}
	nf.watch(nf.TempDir("watched"))
	return nf
}

func (f *notifyFixture) setIgnore(ignore PathMatcher) {
	f.ignore = ignore
	f.rebuildWatcher()
}

func (f *notifyFixture) watch(path string) {
	f.paths = append(f.paths, path)
	f.rebuildWatcher()
}

func (f *notifyFixture) rebuildWatcher() {
	// sync any outstanding events and close the old watcher
	if f.notify != nil {
		f.fsync()
		f.closeWatcher()
	}

	// create a new watcher
	notify, err := NewWatcher(f.paths, f.ignore, logger.NewLogger(logger.DebugLvl, f.out))
	if err != nil {
		f.T().Fatal(err)
	}
	f.notify = notify
	err = f.notify.Start()
	if err != nil {
		f.T().Fatal(err)
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

func (f *notifyFixture) consumeEventsInBackground(ctx context.Context) chan error {
	done := make(chan error)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case err := <-f.notify.Errors():
				done <- err
				close(done)
				return
			case <-f.notify.Events():
			}
		}
	}()
	return done
}

func (f *notifyFixture) fsync() {
	f.fsyncWithRetryCount(3)
}

func (f *notifyFixture) fsyncWithRetryCount(retryCount int) {
	if len(f.paths) == 0 {
		return
	}

	syncPathBase := fmt.Sprintf("sync-%d.txt", time.Now().UnixNano())
	syncPath := filepath.Join(f.paths[0], syncPathBase)
	anySyncPath := filepath.Join(f.paths[0], "sync-")
	timeout := time.After(250 * time.Millisecond)

	f.WriteFile(syncPath, time.Now().String())

F:
	for {
		select {
		case err := <-f.notify.Errors():
			f.T().Fatal(err)

		case event := <-f.notify.Events():
			if strings.Contains(event.Path(), syncPath) {
				break F
			}
			if strings.Contains(event.Path(), anySyncPath) {
				continue
			}

			// Don't bother tracking duplicate changes to the same path
			// for testing.
			if len(f.events) > 0 && f.events[len(f.events)-1].Path() == event.Path() {
				continue
			}

			f.events = append(f.events, event)

		case <-timeout:
			if retryCount <= 0 {
				f.T().Fatalf("fsync: timeout")
			} else {
				f.fsyncWithRetryCount(retryCount - 1)
			}
			return
		}
	}
}

func (f *notifyFixture) closeWatcher() {
	notify := f.notify
	err := notify.Close()
	if err != nil {
		f.T().Fatal(err)
	}

	// drain channels from watcher
	go func() {
		for range notify.Events() {
		}
	}()
	go func() {
		for range notify.Errors() {
		}
	}()
}

func (f *notifyFixture) tearDown() {
	f.closeWatcher()
	f.TempDirFixture.TearDown()
	numberOfWatches.Set(0)
}
