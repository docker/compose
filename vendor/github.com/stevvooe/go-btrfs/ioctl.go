package btrfs

import "syscall"

func ioctl(fd, request, args uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, args)
	if errno != 0 {
		return errno
	}
	return nil
}
