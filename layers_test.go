package containerkit

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLayerManipulatorBasic implements something similar to the conceptual
// examples we've discussed thus far. It does perform mounts, so you must run
// as root.
func TestLayerManipulatorBasic(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test-layman-")
	if err != nil {
		t.Fatal(err)
	}
	// defer os.RemoveAll(tmpDir)

	root := filepath.Join(tmpDir, "root")

	lm, err := NewLayerManipulator(root)
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

		t.Log(MountCommand(mount))
	}

	if err := MountAll(mounts...); err != nil {
		t.Fatal(err)
	}

	if err := ioutil.WriteFile(filepath.Join(preparing, "foo"), []byte("foo\n"), 0777); err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(preparing+"/a/b/c", 0755)

	// defer os.Remove(filepath.Join(tmpDir, "foo"))

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
	if err := MountAll(mounts...); err != nil {
		t.Fatal(err)
	}

	for _, mount := range mounts {
		if !strings.HasPrefix(mount.Target, next) {
			t.Fatalf("expected mount target to be prefixed with tmpDir: %q does not startwith %q", mount.Target, next)
		}

		t.Log(MountCommand(mount))
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
