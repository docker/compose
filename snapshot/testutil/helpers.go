package testutil

import (
	"os/exec"
	"testing"

	"github.com/docker/containerd"
)

func UnmountAll(t *testing.T, mounts []containerd.Mount) {
	for _, mount := range mounts {
		Unmount(t, mount.Target)
	}
}

func Unmount(t *testing.T, mountPoint string) {
	t.Log("unmount", mountPoint)
	umount := exec.Command("umount", mountPoint)
	err := umount.Run()
	if err != nil {

		t.Error("Could not umount", mountPoint, err)
	}
}
