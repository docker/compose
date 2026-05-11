//go:build fsnotify

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
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func newFseventNotifyFixture(repo string, ignore PathMatcher) *fseventNotify {
	return &fseventNotify{
		pathsWereWatching: map[string]any{repo: struct{}{}},
		ignore:            ignore,
	}
}

func TestFseventNotifyCloseIdempotent(t *testing.T) {
	// Create a watcher with a temporary directory
	tmpDir := t.TempDir()
	watcher, err := newWatcher([]string{tmpDir}, nil)
	assert.NilError(t, err)

	// Start the watcher
	err = watcher.Start()
	assert.NilError(t, err)

	// Close should work the first time
	err = watcher.Close()
	assert.NilError(t, err)

	// Close should be idempotent - calling it again should not panic
	err = watcher.Close()
	assert.NilError(t, err)

	// Even a third time should be safe
	err = watcher.Close()
	assert.NilError(t, err)
}

func TestFseventNotifyShouldNotifyNilIgnore(t *testing.T) {
	repo := t.TempDir()
	child := filepath.Join(repo, "child.txt")
	assert.NilError(t, os.WriteFile(child, []byte("x"), 0o644))

	d := newFseventNotifyFixture(repo, nil)
	assert.Assert(t, d.shouldNotify(child), "expected file under watched root to notify")
	assert.Assert(t, !d.shouldNotify(repo), "expected directory event at watched root to be skipped")
}

func TestFseventNotifyShouldNotifyWatchedFileRoot(t *testing.T) {
	repo := t.TempDir()
	fileRoot := filepath.Join(repo, "watched.go")
	assert.NilError(t, os.WriteFile(fileRoot, []byte("package main\n"), 0o644))

	d := newFseventNotifyFixture(fileRoot, nil)
	assert.Assert(t, d.shouldNotify(fileRoot), "expected file that is the watch root to notify")
}

func TestFseventNotifyShouldNotifyOutsideWatchedTree(t *testing.T) {
	repo := t.TempDir()
	other := t.TempDir()

	d := newFseventNotifyFixture(repo, nil)
	outPath := filepath.Join(other, "outside.txt")
	assert.NilError(t, os.WriteFile(outPath, []byte("x"), 0o644))
	assert.Assert(t, !d.shouldNotify(outPath), "expected path outside watch roots not to notify")
}

func TestFseventNotifyShouldNotifyRespectsDockerignore(t *testing.T) {
	repo := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repo, "vendor/\n")
	assert.NilError(t, err)

	d := newFseventNotifyFixture(repo, ignore)
	kept := filepath.Join(repo, "src", "main.go")
	assert.NilError(t, os.MkdirAll(filepath.Dir(kept), 0o755))
	assert.NilError(t, os.WriteFile(kept, []byte("x"), 0o644))
	assert.Assert(t, d.shouldNotify(kept), "expected non-ignored path to notify")

	ignored := filepath.Join(repo, "vendor", "mod", "x.go")
	assert.NilError(t, os.MkdirAll(filepath.Dir(ignored), 0o755))
	assert.NilError(t, os.WriteFile(ignored, []byte("x"), 0o644))
	assert.Assert(t, !d.shouldNotify(ignored), "expected dockerignored path not to notify")
}

func TestFseventNotifyShouldNotifyDockerignoreNegation(t *testing.T) {
	repo := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repo, "bazel-bin/\n!bazel-bin/app-binary\n")
	assert.NilError(t, err)

	d := newFseventNotifyFixture(repo, ignore)

	ignoredChild := filepath.Join(repo, "bazel-bin", "cache", "out")
	assert.NilError(t, os.MkdirAll(filepath.Dir(ignoredChild), 0o755))
	assert.NilError(t, os.WriteFile(ignoredChild, []byte("x"), 0o644))
	assert.Assert(t, !d.shouldNotify(ignoredChild), "expected ignored subtree under bazel-bin not to notify")

	excepted := filepath.Join(repo, "bazel-bin", "app-binary", "binary")
	assert.NilError(t, os.MkdirAll(filepath.Dir(excepted), 0o755))
	assert.NilError(t, os.WriteFile(excepted, []byte("x"), 0o644))
	assert.Assert(t, d.shouldNotify(excepted), "expected negated dockerignore path to notify")
}

func TestFseventNotifyShouldNotifyIntersectMatcher(t *testing.T) {
	repo := t.TempDir()
	ignoreVendor, err := DockerIgnoreTesterFromContents(repo, "vendor/\n")
	assert.NilError(t, err)
	ignoreTmp, err := DockerIgnoreTesterFromContents(repo, "tmp/\n")
	assert.NilError(t, err)

	d := newFseventNotifyFixture(repo, NewIntersectMatcher(ignoreVendor, ignoreTmp))
	vendorFile := filepath.Join(repo, "vendor", "x", "go.mod")
	assert.NilError(t, os.MkdirAll(filepath.Dir(vendorFile), 0o755))
	assert.NilError(t, os.WriteFile(vendorFile, []byte("module x\n"), 0o644))
	assert.Assert(t, d.shouldNotify(vendorFile), "vendor must notify when only one intersect matcher ignores it")

	ignoreBuild1, err := DockerIgnoreTesterFromContents(repo, "build/\n")
	assert.NilError(t, err)
	ignoreBuild2, err := DockerIgnoreTesterFromContents(repo, "build/\n")
	assert.NilError(t, err)
	d2 := newFseventNotifyFixture(repo, NewIntersectMatcher(ignoreBuild1, ignoreBuild2))
	buildFile := filepath.Join(repo, "build", "out", "a")
	assert.NilError(t, os.MkdirAll(filepath.Dir(buildFile), 0o755))
	assert.NilError(t, os.WriteFile(buildFile, []byte("x"), 0o644))
	assert.Assert(t, !d2.shouldNotify(buildFile), "expected path ignored by every intersect matcher not to notify")
}

func TestFseventNotifyShouldIgnoreDockerignoreDirectory(t *testing.T) {
	repo := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repo, "bazel-bin/\n!bazel-bin/app-binary\n")
	assert.NilError(t, err)

	d := newFseventNotifyFixture(repo, ignore)
	bazelBin := filepath.Join(repo, "bazel-bin")
	assert.NilError(t, os.MkdirAll(bazelBin, 0o755))
	assert.Assert(t, d.shouldIgnore(bazelBin), "expected directory path to match dockerignore")
}
