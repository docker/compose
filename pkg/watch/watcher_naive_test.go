// +build !darwin

package watch

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
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
	defer f.tearDown()

	watched := f.TempDir("watched")

	// there are a few different cases we want to test for because the code paths are slightly
	// different:
	// 1) initial: data there before we ever call watch
	// 2) inplace: data we create while the watch is happening
	// 3) staged: data we create in another directory and then atomically move into place

	// initial
	f.WriteFile(f.JoinPath(watched, "initial.txt"), "initial data")

	initialDir := f.JoinPath(watched, "initial_dir")
	if err := os.Mkdir(initialDir, 0777); err != nil {
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
	if err := os.Mkdir(inplace, 0777); err != nil {
		t.Fatal(err)
	}
	f.WriteFile(f.JoinPath(inplace, "inplace.txt"), "inplace data")

	inplaceDir := f.JoinPath(inplace, "inplace_dir")
	if err := os.Mkdir(inplaceDir, 0777); err != nil {
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
	if err := os.Mkdir(stagedDir, 0777); err != nil {
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

	pid := os.Getpid()

	output, err := exec.Command("bash", "-c", fmt.Sprintf(
		"find /proc/%d/fd -lname anon_inode:inotify -printf '%%hinfo/%%f\n' | xargs cat | grep -c '^inotify'", pid)).Output()
	if err != nil {
		t.Fatalf("error running command to determine number of watched files: %v", err)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("couldn't parse number of watched files: %v", err)
	}

	if n > 10 {
		t.Fatalf("watching more than 10 files: %d", n)
	}
}
