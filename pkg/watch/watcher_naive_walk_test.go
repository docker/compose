//go:build !fsnotify

/*
   Copyright 2026 Docker Compose CLI authors

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
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// The walk callbacks have to tolerate parts of the tree the process cannot
// read. A real unreadable directory needs a permission bit root ignores, and
// CI runs as root, so these inject the error instead.

func permissionError(path string) error {
	return &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
}

func dirEntry(t *testing.T, path string) fs.DirEntry {
	t.Helper()
	info, err := os.Lstat(path)
	assert.NilError(t, err)
	return fs.FileInfoToDirEntry(info)
}

func TestWalkAndAddSkipsUnreadableDir(t *testing.T) {
	d := &naiveNotify{}

	err := d.walkAndAdd("/nope", nil, permissionError("/nope"))

	assert.Equal(t, err, filepath.SkipDir)
}

// Anything else is a real failure and has to reach the caller.
func TestWalkAndAddPropagatesOtherWalkErrors(t *testing.T) {
	d := &naiveNotify{}
	boom := errors.New("boom")

	err := d.walkAndAdd("/nope", nil, boom)

	assert.Assert(t, errors.Is(err, boom))
}

// A directory can be listed and still refuse the watch, which is what inotify
// reports for one the process cannot read.
func TestWalkAndAddSkipsDirItCannotWatch(t *testing.T) {
	root := t.TempDir()
	var attempted []string
	d := &naiveNotify{
		notifyList: map[string]bool{root: true},
		addWatch: func(path string) error {
			attempted = append(attempted, path)
			return permissionError(path)
		},
	}

	err := d.walkAndAdd(root, dirEntry(t, root), nil)

	assert.Equal(t, err, filepath.SkipDir)
	assert.DeepEqual(t, attempted, []string{root})
}

// A directory that disappeared mid-walk has nothing left below it to skip.
func TestWalkAndAddIgnoresDirThatDisappeared(t *testing.T) {
	root := t.TempDir()
	d := &naiveNotify{
		notifyList: map[string]bool{root: true},
		addWatch: func(string) error {
			return &fs.PathError{Op: "open", Path: root, Err: fs.ErrNotExist}
		},
	}

	err := d.walkAndAdd(root, dirEntry(t, root), nil)

	assert.NilError(t, err)
}

func TestWalkAndAddPropagatesOtherWatchErrors(t *testing.T) {
	root := t.TempDir()
	d := &naiveNotify{
		notifyList: map[string]bool{root: true},
		addWatch: func(string) error {
			return errors.New("boom")
		},
	}

	err := d.walkAndAdd(root, dirEntry(t, root), nil)

	assert.ErrorContains(t, err, "boom")
}

func TestWalkAndNotifySkipsUnreadableDir(t *testing.T) {
	d := &naiveNotify{}

	err := d.walkAndNotify("/nope")("/nope/inner", nil, permissionError("/nope/inner"))

	assert.Equal(t, err, filepath.SkipDir)
}

func TestWalkAndNotifyPropagatesOtherWalkErrors(t *testing.T) {
	d := &naiveNotify{}
	boom := errors.New("boom")

	err := d.walkAndNotify("/nope")("/nope/inner", nil, boom)

	assert.Assert(t, errors.Is(err, boom))
}
