// +build linux,!arm64

package archutils

import (
	"syscall"
)

func EpollCreate1(flag int) (int, error) {
	return syscall.EpollCreate1(flag)
}

func EpollCtl(epfd int, op int, fd int, event *syscall.EpollEvent) error {
	return syscall.EpollCtl(epfd, op, fd, event)
}

func EpollWait(epfd int, events []syscall.EpollEvent, msec int) (int, error) {
	return syscall.EpollWait(epfd, events, msec)
}
