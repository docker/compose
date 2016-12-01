// +build !solaris

package main

import (
	"syscall"
)

// setPDeathSig sets the parent death signal to SIGKILL so that if the
// shim dies the container process also dies.
func setPDeathSig() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
}
