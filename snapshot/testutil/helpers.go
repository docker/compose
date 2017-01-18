package testutil

import (
	"syscall"
	"testing"
)

func Unmount(t *testing.T, mountPoint string) {
	t.Log("unmount", mountPoint)
	if err := syscall.Unmount(mountPoint, 0); err != nil {
		t.Error("Could not umount", mountPoint, err)
	}
}
