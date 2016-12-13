package snapshot

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/containerd"
)

func TestBtrfs(t *testing.T) {
	// SORRY(stevvooe): This is where I mount a btrfs loopback. We can probably
	// set this up as part of the test.
	root, err := ioutil.TempDir("/tmp/snapshots", "TestBtrfsPrepare-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	// TODO(stevvooe): Cleanup subvolumes

	sm, err := NewBtrfs("/dev/loop0", root)
	if err != nil {
		t.Fatal(err)
	}
	mounts, err := sm.Prepare(filepath.Join(root, "test"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(mounts)

	for _, mount := range mounts {
		if mount.Type != "btrfs" {
			t.Fatal("wrong mount type: %v != btrfs", mount.Type)
		}

		// assumes the first, maybe incorrect in the future
		if !strings.HasPrefix(mount.Options[0], "subvolid=") {
			t.Fatal("no subvolid option in %v", mount.Options)
		}
	}

	if err := os.MkdirAll(mounts[0].Target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := containerd.MountAll(mounts...); err != nil {
		t.Fatal(err)
	}

	// write in some data
	if err := ioutil.WriteFile(filepath.Join(mounts[0].Target, "foo"), []byte("content"), 0777); err != nil {
		t.Fatal(err)
	}

	// TODO(stevvooe): We don't really make this with the driver, but that
	// might prove annoying in practice.
	if err := os.MkdirAll(filepath.Join(root, "snapshots"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := sm.Commit(filepath.Join(root, "snapshots/committed"), filepath.Join(root, "test")); err != nil {
		t.Fatal(err)
	}

	mounts, err = sm.Prepare(filepath.Join(root, "test2"), filepath.Join(root, "snapshots/committed"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(root, "test2"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := containerd.MountAll(mounts...); err != nil {
		t.Fatal(err)
	}

	// TODO(stevvooe): Verify contents of "foo"
	if err := ioutil.WriteFile(filepath.Join(mounts[0].Target, "bar"), []byte("content"), 0777); err != nil {
		t.Fatal(err)
	}

	if err := sm.Commit(filepath.Join(root, "snapshots/committed2"), filepath.Join(root, "test2")); err != nil {
		t.Fatal(err)
	}
}
