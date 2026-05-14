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
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGreatestExistingAncestor(t *testing.T) {
	f := NewTempDirFixture(t)

	p, err := greatestExistingAncestor(f.Path())
	assert.NilError(t, err)
	assert.Equal(t, f.Path(), p)

	p, err = greatestExistingAncestor(f.JoinPath("missing"))
	assert.NilError(t, err)
	assert.Equal(t, f.Path(), p)

	missingTopLevel := "/missingDir/a/b/c"
	if runtime.GOOS == "windows" {
		missingTopLevel = `C:\missingDir\a\b\c`
	}
	_, err = greatestExistingAncestor(missingTopLevel)
	assert.ErrorContains(t, err, "cannot watch root directory")
}

func TestGreatestExistingAncestorsMovesIgnoreToAncestor(t *testing.T) {
	f := NewTempDirFixture(t)

	missing := f.JoinPath("missing", "child", "file.txt")
	ignore, err := DockerIgnoreTesterFromContents(f.Path(), "vendor/\n")
	assert.NilError(t, err)

	ignoreList := map[string]PathMatcher{missing: ignore}
	paths, err := greatestExistingAncestors([]string{missing}, ignoreList)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(paths))
	assert.Equal(t, f.Path(), paths[0])
	assert.Assert(t, ignoreList[f.Path()] != nil)
	_, exists := ignoreList[missing]
	assert.Assert(t, !exists)
}

func TestGreatestExistingAncestorsIntersectsIgnoreOnAncestor(t *testing.T) {
	f := NewTempDirFixture(t)

	missing := f.JoinPath("missing", "child", "file.txt")
	vendorIgnore, err := DockerIgnoreTesterFromContents(f.Path(), "vendor/\n")
	assert.NilError(t, err)
	tmpIgnore, err := DockerIgnoreTesterFromContents(f.Path(), "tmp/\n")
	assert.NilError(t, err)

	ignoreList := map[string]PathMatcher{
		f.Path(): vendorIgnore,
		missing:  tmpIgnore,
	}
	paths, err := greatestExistingAncestors([]string{missing}, ignoreList)
	assert.NilError(t, err)
	assert.Equal(t, 1, len(paths))
	assert.Equal(t, f.Path(), paths[0])

	inter, ok := ignoreList[f.Path()].(intersectPathMatcher)
	assert.Assert(t, ok)
	assert.Equal(t, 2, len(inter.Matchers))
}

func TestNormalizeWatchRootsAbsolutizesPaths(t *testing.T) {
	rel := "."
	abs, err := filepath.Abs(rel)
	assert.NilError(t, err)

	notifyList, _, err := normalizeWatchRoots([]string{rel}, nil)
	assert.NilError(t, err)
	assert.Assert(t, notifyList[abs])
}

func TestNormalizeWatchRootsAssignsRelatedIgnores(t *testing.T) {
	f := NewTempDirFixture(t)

	root := f.Path()
	child := f.JoinPath("child")
	vendorIgnore, err := DockerIgnoreTesterFromContents(root, "vendor/\n")
	assert.NilError(t, err)
	unrelatedIgnore, err := DockerIgnoreTesterFromContents(root, "build/\n")
	assert.NilError(t, err)

	ignores := map[string]PathMatcher{
		root:     vendorIgnore,
		child:    vendorIgnore,
		"/other": unrelatedIgnore,
	}
	notifyList, normalizedIgnores, err := normalizeWatchRoots([]string{root, child}, ignores)
	assert.NilError(t, err)
	assert.Assert(t, notifyList[root])
	assert.Assert(t, notifyList[child])

	vendorFile := filepath.Join(root, "vendor", "mod.go")
	matches, err := normalizedIgnores[root].Matches(vendorFile)
	assert.NilError(t, err)
	assert.Assert(t, matches)

	matches, err = normalizedIgnores[child].Matches(vendorFile)
	assert.NilError(t, err)
	assert.Assert(t, matches)

	buildFile := filepath.Join(root, "build", "out")
	matches, err = normalizedIgnores[root].Matches(buildFile)
	assert.NilError(t, err)
	assert.Assert(t, !matches)
}

func TestNormalizeWatchRootsSkipsNilMatchers(t *testing.T) {
	f := NewTempDirFixture(t)

	root := f.Path()
	notifyList, normalizedIgnores, err := normalizeWatchRoots([]string{root}, map[string]PathMatcher{root: nil})
	assert.NilError(t, err)
	assert.Assert(t, notifyList[root])
	_, ok := normalizedIgnores[root].(EmptyMatcher)
	assert.Assert(t, ok)
}

func TestNormalizeWatchRootsUsesEmptyMatcherWithoutIgnores(t *testing.T) {
	f := NewTempDirFixture(t)

	root := f.Path()
	_, normalizedIgnores, err := normalizeWatchRoots([]string{root}, nil)
	assert.NilError(t, err)
	_, ok := normalizedIgnores[root].(EmptyMatcher)
	assert.Assert(t, ok)
}

func TestNormalizeWatchRootsInheritsParentIgnoreForChild(t *testing.T) {
	f := NewTempDirFixture(t)

	root := f.Path()
	child := f.JoinPath("pkg")
	vendorIgnore, err := DockerIgnoreTesterFromContents(root, "pkg/vendor/\n")
	assert.NilError(t, err)

	_, normalizedIgnores, err := normalizeWatchRoots([]string{child}, map[string]PathMatcher{root: vendorIgnore})
	assert.NilError(t, err)

	vendorFile := filepath.Join(child, "vendor", "x.go")
	matches, err := normalizedIgnores[child].Matches(vendorFile)
	assert.NilError(t, err)
	assert.Assert(t, matches)
}

func TestNormalizeWatchRootsIntersectsNestedIgnores(t *testing.T) {
	f := NewTempDirFixture(t)

	root := f.Path()
	child := f.JoinPath("pkg")
	vendorIgnore, err := DockerIgnoreTesterFromContents(root, "vendor/\n")
	assert.NilError(t, err)
	tmpIgnore, err := DockerIgnoreTesterFromContents(root, "pkg/tmp/\n")
	assert.NilError(t, err)

	ignores := map[string]PathMatcher{
		root:  vendorIgnore,
		child: tmpIgnore,
	}
	_, normalizedIgnores, err := normalizeWatchRoots([]string{root, child}, ignores)
	assert.NilError(t, err)

	vendorFile := filepath.Join(root, "vendor", "x.go")
	matches, err := normalizedIgnores[root].Matches(vendorFile)
	assert.NilError(t, err)
	assert.Assert(t, !matches, "nested ignores must all match for parent root")

	tmpUnderChild := filepath.Join(child, "tmp", "a")
	matches, err = normalizedIgnores[child].Matches(tmpUnderChild)
	assert.NilError(t, err)
	assert.Assert(t, !matches, "nested ignores must all match for child root")
}
