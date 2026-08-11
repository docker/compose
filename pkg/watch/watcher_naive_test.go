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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestDontWatchEachFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("This test uses linux-specific inotify checks")
	}

	// fsnotify is not recursive, so we need to watch each directory
	// you can watch individual files with fsnotify, but that is more prone to exhaust resources
	// this test uses a Linux way to get the number of watches to make sure we're watching
	// per-directory, not per-file
	f := newNotifyFixture(t)

	watched := f.TempDir("watched")

	// there are a few different cases we want to test for because the code paths are slightly
	// different:
	// 1) initial: data there before we ever call watch
	// 2) inplace: data we create while the watch is happening
	// 3) staged: data we create in another directory and then atomically move into place

	// initial
	f.WriteFile(f.JoinPath(watched, "initial.txt"), "initial data")

	initialDir := f.JoinPath(watched, "initial_dir")
	if err := os.Mkdir(initialDir, 0o777); err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		f.WriteFile(f.JoinPath(initialDir, fmt.Sprintf("%d", i)), "initial data")
	}

	f.watch(watched)
	f.fsync()
	if len(f.events) != 0 {
		t.Fatalf("expected 0 initial events; got %d events: %v", len(f.events), f.events)
	}
	f.events = nil

	// inplace
	inplace := f.JoinPath(watched, "inplace")
	if err := os.Mkdir(inplace, 0o777); err != nil {
		t.Fatal(err)
	}
	f.WriteFile(f.JoinPath(inplace, "inplace.txt"), "inplace data")

	inplaceDir := f.JoinPath(inplace, "inplace_dir")
	if err := os.Mkdir(inplaceDir, 0o777); err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		f.WriteFile(f.JoinPath(inplaceDir, fmt.Sprintf("%d", i)), "inplace data")
	}

	f.fsync()
	if len(f.events) < 100 {
		t.Fatalf("expected >100 inplace events; got %d events: %v", len(f.events), f.events)
	}
	f.events = nil

	// staged
	staged := f.TempDir("staged")
	f.WriteFile(f.JoinPath(staged, "staged.txt"), "staged data")

	stagedDir := f.JoinPath(staged, "staged_dir")
	if err := os.Mkdir(stagedDir, 0o777); err != nil {
		t.Fatal(err)
	}

	for i := range 100 {
		f.WriteFile(f.JoinPath(stagedDir, fmt.Sprintf("%d", i)), "staged data")
	}

	if err := os.Rename(staged, f.JoinPath(watched, "staged")); err != nil {
		t.Fatal(err)
	}

	f.fsync()
	if len(f.events) < 100 {
		t.Fatalf("expected >100 staged events; got %d events: %v", len(f.events), f.events)
	}
	f.events = nil

	n, err := inotifyNodes()
	assert.NilError(t, err)
	if n > 10 {
		t.Fatalf("watching more than 10 files: %d", n)
	}
}

func inotifyNodes() (int, error) {
	pid := os.Getpid()

	output, err := exec.Command("/bin/sh", "-c", fmt.Sprintf(
		"find /proc/%d/fd -lname anon_inode:inotify -printf '%%hinfo/%%f\n' | xargs cat | grep -c '^inotify'", pid,
	)).Output()
	if err != nil {
		return 0, fmt.Errorf("error running command to determine number of watched files: %w\n %s", err, output)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("couldn't parse number of watched files: %w", err)
	}
	return n, nil
}

func TestDontRecurseWhenWatchingParentsOfNonExistentFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("This test uses linux-specific inotify checks")
	}

	f := newNotifyFixture(t)

	watched := f.TempDir("watched")
	f.watch(filepath.Join(watched, ".tiltignore"))

	excludedDir := f.JoinPath(watched, "excluded")
	for i := range 10 {
		f.WriteFile(f.JoinPath(excludedDir, fmt.Sprintf("%d", i), "data.txt"), "initial data")
	}
	f.fsync()

	n, err := inotifyNodes()
	assert.NilError(t, err)
	if n > 5 {
		t.Fatalf("watching more than 5 files: %d", n)
	}
}

func TestShouldSkipDirWithNegatedChildException(t *testing.T) {
	repoRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "bazel-bin/\n!bazel-bin/app-binary\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: ignore},
		notifyList: map[string]bool{repoRoot: true},
	}

	bazelBin := filepath.Join(repoRoot, "bazel-bin")
	assert.Assert(t, !d.shouldSkipDir(bazelBin), "expected bazel-bin to remain traversable for negated child patterns")
}

func TestShouldIgnorePathStillMatchesDirectoryPattern(t *testing.T) {
	repoRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "bazel-bin/\n!bazel-bin/app-binary\n")
	assert.NilError(t, err)

	d := &naiveNotify{ignore: map[string]PathMatcher{repoRoot: ignore}}

	bazelBin := filepath.Join(repoRoot, "bazel-bin")
	assert.Assert(t, d.shouldIgnore(repoRoot, bazelBin), "expected bazel-bin path to match ignore pattern")
}

func TestShouldIgnoreLooksUpMatcherByWatchRoot(t *testing.T) {
	repoRoot := t.TempDir()
	otherRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "vendor/\n")
	assert.NilError(t, err)

	d := &naiveNotify{ignore: map[string]PathMatcher{repoRoot: ignore}}

	vendorFile := filepath.Join(repoRoot, "vendor", "x.go")
	assert.Assert(t, d.shouldIgnore(repoRoot, vendorFile), "expected matcher keyed to watched root to apply")
	assert.Assert(t, !d.shouldIgnore(otherRoot, vendorFile), "expected unrelated watch root not to apply matcher")
}

func TestShouldIgnoreEntireDirLooksUpMatcherByWatchRoot(t *testing.T) {
	repoRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "vendor/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: ignore},
		notifyList: map[string]bool{repoRoot: true},
	}

	vendorDir := filepath.Join(repoRoot, "vendor")
	assert.Assert(t, d.shouldIgnoreEntireDir(repoRoot, vendorDir), "expected directory matcher keyed to watched root to apply")
}

func TestShouldSkipDirForIgnoredSubtreeWithoutException(t *testing.T) {
	repoRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "bazel-bin/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: ignore},
		notifyList: map[string]bool{repoRoot: true},
	}

	bazelBin := filepath.Join(repoRoot, "bazel-bin")
	assert.Assert(t, d.shouldSkipDir(bazelBin), "expected fully ignored directory subtree to be skipped")
}

func TestShouldSkipDirDoesNotSkipAncestorOfWatchedPath(t *testing.T) {
	repoRoot := t.TempDir()
	ignore, err := DockerIgnoreTesterFromContents(repoRoot, "parent/\n")
	assert.NilError(t, err)

	watchedPath := filepath.Join(repoRoot, "parent", "child", "non-existent")
	d := &naiveNotify{
		ignore:     map[string]PathMatcher{watchedPath: ignore},
		notifyList: map[string]bool{watchedPath: true},
	}

	parent := filepath.Join(repoRoot, "parent")
	assert.Assert(t, !d.shouldSkipDir(parent), "expected parent directory to remain traversable when it contains a watched path")
}

func TestShouldSkipDirRequiresAllContainingRootsToAgree(t *testing.T) {
	repoRoot := t.TempDir()
	srcRoot := filepath.Join(repoRoot, "src")

	// Service A watches repoRoot but does NOT list node_modules in its ignores.
	// Service B watches src and ignores node_modules.
	// node_modules under src must NOT be skipped — because service A (which also
	// covers the path) has no rule for it. All containing roots must agree to skip.
	rootIgnore, err := DockerIgnoreTesterFromContents(repoRoot, "need_perm_dir/\n")
	assert.NilError(t, err)
	childIgnore, err := DockerIgnoreTesterFromContents(srcRoot, "node_modules/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: rootIgnore, srcRoot: childIgnore},
		notifyList: map[string]bool{repoRoot: true, srcRoot: true},
	}

	nodeModulesDir := filepath.Join(srcRoot, "node_modules")
	assert.Assert(t, !d.shouldSkipDir(nodeModulesDir),
		"node_modules under child root must not be skipped when a containing root (repoRoot) has no matching ignore rule")

	// A legitimate subdir under src is also not skipped.
	componentsDir := filepath.Join(srcRoot, "components")
	assert.Assert(t, !d.shouldSkipDir(componentsDir),
		"non-ignored directory under child root must remain watched")
}

func TestShouldSkipDirNotVetoedByUnrelatedChildTrigger(t *testing.T) {
	repoRoot := t.TempDir()
	srcRoot := filepath.Join(repoRoot, "src")

	// Service A watches repoRoot and ignores root-owned-dir/.
	// Service B watches repoRoot/src with an unrelated ignore.
	// root-owned-dir is outside src, so service B has no opinion about it.
	// The directory must still be skipped so the walker never enters it.
	rootIgnore, err := DockerIgnoreTesterFromContents(repoRoot, "root-owned-dir/\n")
	assert.NilError(t, err)
	childIgnore, err := DockerIgnoreTesterFromContents(srcRoot, "node_modules/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: rootIgnore, srcRoot: childIgnore},
		notifyList: map[string]bool{repoRoot: true, srcRoot: true},
	}

	rootOwnedDir := filepath.Join(repoRoot, "root-owned-dir")
	assert.Assert(t, d.shouldSkipDir(rootOwnedDir),
		"root-owned-dir must be skipped; child trigger must not veto parent's ignore")
}

func TestShouldNotifyAnyRootSaysOK(t *testing.T) {
	repoRoot := t.TempDir()
	srcRoot := filepath.Join(repoRoot, "src")

	// Service A watches repoRoot and ignores the entire src/ subtree.
	// Service B watches repoRoot/src and ignores only node_modules/.
	// A path is notified if ANY containing root's matcher does not suppress it.
	parentIgnore, err := DockerIgnoreTesterFromContents(repoRoot, "src/\n")
	assert.NilError(t, err)
	childIgnore, err := DockerIgnoreTesterFromContents(srcRoot, "node_modules/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: parentIgnore, srcRoot: childIgnore},
		notifyList: map[string]bool{repoRoot: true, srcRoot: true},
	}

	// A regular source file under src/ is notified because srcRoot's matcher does
	// not ignore it, even though repoRoot ignores all of src/.
	fooFile := filepath.Join(srcRoot, "foo.ts")
	assert.Assert(t, d.shouldNotify(fooFile),
		"file under child root must be notified; srcRoot does not ignore it even though repoRoot ignores src/")

	// A file inside node_modules is not notified: every containing root ignores it
	// (repoRoot ignores src/, srcRoot ignores node_modules/).
	nodeModulesFile := filepath.Join(srcRoot, "node_modules", "dep.js")
	assert.Assert(t, !d.shouldNotify(nodeModulesFile),
		"node_modules file must not be notified; all containing roots ignore it")

	// A file outside src/ is notified because repoRoot does not ignore it.
	otherFile := filepath.Join(repoRoot, "main.go")
	assert.Assert(t, d.shouldNotify(otherFile),
		"file outside src/ must be notified; repoRoot does not ignore it")
}

func TestShouldSkipDirIntersectRequiresAllTriggersToAgree(t *testing.T) {
	repoRoot := t.TempDir()
	ignoreVendor, err := DockerIgnoreTesterFromContents(repoRoot, "vendor/\n")
	assert.NilError(t, err)
	ignoreTmp, err := DockerIgnoreTesterFromContents(repoRoot, "tmp/\n")
	assert.NilError(t, err)

	d := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: NewIntersectMatcher(ignoreVendor, ignoreTmp)},
		notifyList: map[string]bool{repoRoot: true},
	}

	vendorDir := filepath.Join(repoRoot, "vendor")
	assert.Assert(t, !d.shouldSkipDir(vendorDir), "vendor must remain watched when another trigger does not ignore it")

	ignoreBuild1, err := DockerIgnoreTesterFromContents(repoRoot, "build/\n")
	assert.NilError(t, err)
	ignoreBuild2, err := DockerIgnoreTesterFromContents(repoRoot, "build/\n")
	assert.NilError(t, err)
	d2 := &naiveNotify{
		ignore:     map[string]PathMatcher{repoRoot: NewIntersectMatcher(ignoreBuild1, ignoreBuild2)},
		notifyList: map[string]bool{repoRoot: true},
	}
	buildDir := filepath.Join(repoRoot, "build")
	assert.Assert(t, d2.shouldSkipDir(buildDir), "when every trigger ignores the same subtree, watcher may skip it")
}
