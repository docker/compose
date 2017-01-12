package snapshot

import (
	"os/exec"
	"testing"

	"github.com/docker/containerd"
)

func unmountAll(t *testing.T, mounts []containerd.Mount) {
	for _, mount := range mounts {
		unmount(t, mount.Target)
	}
}

func unmount(t *testing.T, mountPoint string) {
	t.Log("unmount", mountPoint)
	umount := exec.Command("umount", mountPoint)
	err := umount.Run()
	if err != nil {

		t.Error("Could not umount", mountPoint, err)
	}
}
