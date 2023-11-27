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
	"errors"
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/tilt-dev/fsnotify"
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
// The PathMatcher interface helps us manage these ignores.
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

func NewWatcher(paths []string, ignore PathMatcher) (Notify, error) {
	return newWatcher(paths, ignore)
}

const WindowsBufferSizeEnvVar = "COMPOSE_WATCH_WINDOWS_BUFFER_SIZE"

const defaultBufferSize int = 65536

func DesiredWindowsBufferSize() int {
	envVar := os.Getenv(WindowsBufferSizeEnvVar)
	if envVar != "" {
		size, err := strconv.Atoi(envVar)
		if err == nil {
			return size
		}
	}
	return defaultBufferSize
}

func IsWindowsShortReadError(err error) bool {
	return runtime.GOOS == "windows" && !errors.Is(err, fsnotify.ErrEventOverflow)
}

type CompositePathMatcher struct {
	Matchers []PathMatcher
}

func NewCompositeMatcher(matchers ...PathMatcher) PathMatcher {
	if len(matchers) == 0 {
		return EmptyMatcher{}
	}
	return CompositePathMatcher{Matchers: matchers}
}

func (c CompositePathMatcher) Matches(f string) (bool, error) {
	for _, t := range c.Matchers {
		ret, err := t.Matches(f)
		if err != nil {
			return false, err
		}
		if ret {
			return true, nil
		}
	}
	return false, nil
}

func (c CompositePathMatcher) MatchesEntireDir(f string) (bool, error) {
	for _, t := range c.Matchers {
		matches, err := t.MatchesEntireDir(f)
		if matches || err != nil {
			return matches, err
		}
	}
	return false, nil
}

var _ PathMatcher = CompositePathMatcher{}
