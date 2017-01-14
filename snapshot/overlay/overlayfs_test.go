package overlay

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/docker/containerd"
)

func TestOverlayfs(t *testing.T) {
	root, err := ioutil.TempDir("", "overlay")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	o, err := NewOverlayfs(root)
	if err != nil {
		t.Error(err)
		return
	}
	mounts, err := o.Prepare("/tmp/test", "")
	if err != nil {
		t.Error(err)
		return
	}
	if len(mounts) != 1 {
		t.Errorf("should only have 1 mount but received %d", len(mounts))
	}
	m := mounts[0]
	if m.Type != "bind" {
		t.Errorf("mount type should be bind but received %q", m.Type)
	}
	expected := filepath.Join(root, "active", hash("/tmp/test"), "fs")
	if m.Source != expected {
		t.Errorf("expected source %q but received %q", expected, m.Source)
	}
	if m.Options[0] != "rw" {
		t.Errorf("expected mount option rw but received %q", m.Options[0])
	}
	if m.Options[1] != "rbind" {
		t.Errorf("expected mount option rbind but received %q", m.Options[1])
	}
}

func TestOverlayfsCommit(t *testing.T) {
	root, err := ioutil.TempDir("", "overlay")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	o, err := NewOverlayfs(root)
	if err != nil {
		t.Error(err)
		return
	}
	key := "/tmp/test"
	mounts, err := o.Prepare(key, "")
	if err != nil {
		t.Error(err)
		return
	}
	m := mounts[0]
	if err := ioutil.WriteFile(filepath.Join(m.Source, "foo"), []byte("hi"), 0660); err != nil {
		t.Error(err)
		return
	}
	if err := o.Commit("base", key); err != nil {
		t.Error(err)
		return
	}
}

func TestOverlayfsOverlayMount(t *testing.T) {
	root, err := ioutil.TempDir("", "overlay")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	o, err := NewOverlayfs(root)
	if err != nil {
		t.Error(err)
		return
	}
	key := "/tmp/test"
	mounts, err := o.Prepare(key, "")
	if err != nil {
		t.Error(err)
		return
	}
	if err := o.Commit("base", key); err != nil {
		t.Error(err)
		return
	}
	if mounts, err = o.Prepare("/tmp/layer2", "base"); err != nil {
		t.Error(err)
		return
	}
	if len(mounts) != 1 {
		t.Errorf("should only have 1 mount but received %d", len(mounts))
	}
	m := mounts[0]
	if m.Type != "overlay" {
		t.Errorf("mount type should be overlay but received %q", m.Type)
	}
	if m.Source != "overlay" {
		t.Errorf("expected source %q but received %q", "overlay", m.Source)
	}
	var (
		hash  = hash("/tmp/layer2")
		work  = "workdir=" + filepath.Join(root, "active", hash, "work")
		upper = "upperdir=" + filepath.Join(root, "active", hash, "fs")
		lower = "lowerdir=" + filepath.Join(root, "snapshots", "base", "fs")
	)
	for i, v := range []string{
		work,
		upper,
		lower,
	} {
		if m.Options[i] != v {
			t.Errorf("expected %q but received %q", v, m.Options[i])
		}
	}
}

func TestOverlayfsOverlayRead(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("not running as root")
	}
	root, err := ioutil.TempDir("", "overlay")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	o, err := NewOverlayfs(root)
	if err != nil {
		t.Error(err)
		return
	}
	key := "/tmp/test"
	mounts, err := o.Prepare(key, "")
	if err != nil {
		t.Error(err)
		return
	}
	m := mounts[0]
	if err := ioutil.WriteFile(filepath.Join(m.Source, "foo"), []byte("hi"), 0660); err != nil {
		t.Error(err)
		return
	}
	if err := o.Commit("base", key); err != nil {
		t.Error(err)
		return
	}
	if mounts, err = o.Prepare("/tmp/layer2", "base"); err != nil {
		t.Error(err)
		return
	}
	dest := filepath.Join(root, "dest")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Error(err)
		return
	}
	if err := containerd.MountFS(mounts, dest); err != nil {
		t.Error(err)
		return
	}
	defer syscall.Unmount(dest, 0)
	data, err := ioutil.ReadFile(filepath.Join(dest, "foo"))
	if err != nil {
		t.Error(err)
		return
	}
	if e := string(data); e != "hi" {
		t.Errorf("expected file contents hi but got %q", e)
		return
	}
}
