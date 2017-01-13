package snapshot

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/containerd"
	"github.com/docker/containerd/snapshot/testutil"
)

// TestSnapshotManagerBasic implements something similar to the conceptual
// examples we've discussed thus far. It does perform mounts, so you must run
// as root.
func TestSnapshotManagerBasic(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test-layman-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("Removing", tmpDir)
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Error(err)
		}
	}()

	root := filepath.Join(tmpDir, "root")

	lm, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}

	preparing := filepath.Join(tmpDir, "preparing")
	if err := os.MkdirAll(preparing, 0777); err != nil {
		t.Fatal(err)
	}

	mounts, err := lm.Prepare(preparing, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(mounts) < 1 {
		t.Fatal("expected mounts to have entries")
	}

	for _, mount := range mounts {
		if !strings.HasPrefix(mount.Target, preparing) {
			t.Fatalf("expected mount target to be prefixed with tmpDir: %q does not startwith %q", mount.Target, preparing)
		}

		t.Log(containerd.MountCommand(mount))
	}

	if err := containerd.MountAll(mounts...); err != nil {
		t.Fatal(err)
	}
	defer testutil.UnmountAll(t, mounts)

	if err := ioutil.WriteFile(filepath.Join(preparing, "foo"), []byte("foo\n"), 0777); err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(preparing+"/a/b/c", 0755)

	committed := filepath.Join(lm.root, "committed")

	if err := lm.Commit(committed, preparing); err != nil {
		t.Fatal(err)
	}

	if lm.Parent(preparing) != "" {
		t.Fatalf("parent of new layer should be empty, got lm.Parent(%q) == %q", preparing, lm.Parent(preparing))
	}

	next := filepath.Join(tmpDir, "nextlayer")
	if err := os.MkdirAll(next, 0777); err != nil {
		t.Fatal(err)
	}

	mounts, err = lm.Prepare(next, committed)
	if err != nil {
		t.Fatal(err)
	}
	if err := containerd.MountAll(mounts...); err != nil {
		t.Fatal(err)
	}
	defer testutil.UnmountAll(t, mounts)

	for _, mount := range mounts {
		if !strings.HasPrefix(mount.Target, next) {
			t.Fatalf("expected mount target to be prefixed with tmpDir: %q does not startwith %q", mount.Target, next)
		}

		t.Log(containerd.MountCommand(mount))
	}

	if err := ioutil.WriteFile(filepath.Join(next, "bar"), []byte("bar\n"), 0777); err != nil {
		t.Fatal(err)
	}

	// also, change content of foo to bar
	if err := ioutil.WriteFile(filepath.Join(next, "foo"), []byte("bar\n"), 0777); err != nil {
		t.Fatal(err)
	}

	os.RemoveAll(next + "/a/b")
	nextCommitted := filepath.Join(lm.root, "committed-next")
	if err := lm.Commit(nextCommitted, next); err != nil {
		t.Fatal(err)
	}

	if lm.Parent(nextCommitted) != committed {
		t.Fatalf("parent of new layer should be %q, got lm.Parent(%q) == %q (%#v)", committed, next, lm.Parent(next), lm.parents)
	}
}
