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

	"github.com/stretchr/testify/require"
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

	for i := 0; i < 100; i++ {
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

	for i := 0; i < 100; i++ {
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

	for i := 0; i < 100; i++ {
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
	require.NoError(t, err)
	if n > 10 {
		t.Fatalf("watching more than 10 files: %d", n)
	}
}

func inotifyNodes() (int, error) {
	pid := os.Getpid()

	output, err := exec.Command("/bin/sh", "-c", fmt.Sprintf(
		"find /proc/%d/fd -lname anon_inode:inotify -printf '%%hinfo/%%f\n' | xargs cat | grep -c '^inotify'", pid)).Output()
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
	for i := 0; i < 10; i++ {
		f.WriteFile(f.JoinPath(excludedDir, fmt.Sprintf("%d", i), "data.txt"), "initial data")
	}
	f.fsync()

	n, err := inotifyNodes()
	require.NoError(t, err)
	if n > 5 {
		t.Fatalf("watching more than 5 files: %d", n)
	}
}
