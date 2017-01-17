// +build !solaris

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// NewConsole returns an initialized console that can be used within a container by copying bytes
// from the master side to the slave that is attached as the tty for the container's init process.
func newConsole(uid, gid int) (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	if err = saneTerminal(master); err != nil {
		return nil, "", err
	}
	console, err := ptsname(master)
	if err != nil {
		return nil, "", err
	}
	if err := unlockpt(master); err != nil {
		return nil, "", err
	}
	if err := os.Chmod(console, 0600); err != nil {
		return nil, "", err
	}
	if err := os.Chown(console, uid, gid); err != nil {
		return nil, "", err
	}
	return master, console, nil
}

// saneTerminal sets the necessary tty_ioctl(4)s to ensure that a pty pair
// created by us acts normally. In particular, a not-very-well-known default of
// Linux unix98 ptys is that they have +onlcr by default. While this isn't a
// problem for terminal emulators, because we relay data from the terminal we
// also relay that funky line discipline.
func saneTerminal(terminal *os.File) error {
	// Go doesn't have a wrapper for any of the termios ioctls.
	var termios syscall.Termios

	if err := ioctl(terminal.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios))); err != nil {
		return fmt.Errorf("ioctl(tty, tcgets): %s", err.Error())
	}

	// Set -onlcr so we don't have to deal with \r.
	termios.Oflag &^= syscall.ONLCR

	if err := ioctl(terminal.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&termios))); err != nil {
		return fmt.Errorf("ioctl(tty, tcsets): %s", err.Error())
	}

	return nil
}

func ioctl(fd uintptr, flag, data uintptr) error {
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, flag, data); err != 0 {
		return err
	}
	return nil
}

// unlockpt unlocks the slave pseudoterminal device corresponding to the master pseudoterminal referred to by f.
// unlockpt should be called before opening the slave side of a pty.
func unlockpt(f *os.File) error {
	var u int32
	return ioctl(f.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
}

// ptsname retrieves the name of the first available pts for the given master.
func ptsname(f *os.File) (string, error) {
	var n int32
	if err := ioctl(f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
