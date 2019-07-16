package watch

import (
	"expvar"
	"fmt"
	"path/filepath"

	"github.com/windmilleng/tilt/internal/logger"
)

var (
	numberOfWatches = expvar.NewInt("watch.naive.numberOfWatches")
)

type FileEvent struct {
	path string
}

func NewFileEvent(p string) FileEvent {
	if !filepath.IsAbs(p) {
		panic(fmt.Sprintf("NewFileEvent only accepts absolute paths. Actual: %s", p))
	}
	return FileEvent{path: p}
}

func (e FileEvent) Path() string {
	return e.path
}

type Notify interface {
	// Start watching the paths set at init time
	Start() error

	// Stop watching and close all channels
	Close() error

	// A channel to read off incoming file changes
	Events() chan FileEvent

	// A channel to read off show-stopping errors
	Errors() chan error
}

// When we specify directories to watch, we often want to
// ignore some subset of the files under those directories.
//
// For example:
// - Watch /src/repo, but ignore /src/repo/.git
// - Watch /src/repo, but ignore everything in /src/repo/bazel-bin except /src/repo/bazel-bin/app-binary
//
// The PathMatcher inteface helps us manage these ignores.
type PathMatcher interface {
	Matches(file string) (bool, error)

	// If this matches the entire dir, we can often optimize filetree walks a bit.
	MatchesEntireDir(file string) (bool, error)
}

type EmptyMatcher struct {
}

func (EmptyMatcher) Matches(f string) (bool, error)          { return false, nil }
func (EmptyMatcher) MatchesEntireDir(f string) (bool, error) { return false, nil }

var _ PathMatcher = EmptyMatcher{}

func NewWatcher(paths []string, ignore PathMatcher, l logger.Logger) (Notify, error) {
	return newWatcher(paths, ignore, l)
}
