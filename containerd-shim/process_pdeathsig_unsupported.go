// +build solaris

package main

import (
	"syscall"
)

// setPDeathSig is a no-op on Solaris as Pdeathsig is not defined.
func setPDeathSig() *syscall.SysProcAttr {
	return nil
}
