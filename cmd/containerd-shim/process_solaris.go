// +build solaris

package main

import (
	"io"
	"os"
	"syscall"
)

// setPDeathSig is a no-op on Solaris as Pdeathsig is not defined.
func setPDeathSig() *syscall.SysProcAttr {
	return nil
}

// TODO: Update to using fifo's package in openIO. Need to
// 1. Merge and vendor changes in the package to use sys/unix.
// 2. Figure out why context.Background is timing out.
// openIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func (p *process) openIO() error {
	p.stdio = &stdio{}
	var (
		uid = p.state.RootUID
	)
	i, err := p.initializeIO(uid)
	if err != nil {
		return err
	}
	p.shimIO = i
	// Both tty and non-tty mode are handled by the runtime using
	// the following pipes
	for name, dest := range map[string]func(f *os.File){
		p.state.Stdout: func(f *os.File) {
			p.Add(1)
			go func() {
				io.Copy(f, i.Stdout)
				p.Done()
			}()
		},
		p.state.Stderr: func(f *os.File) {
			p.Add(1)
			go func() {
				io.Copy(f, i.Stderr)
				p.Done()
			}()
		},
	} {
		f, err := os.OpenFile(name, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		dest(f)
	}

	f, err := os.OpenFile(p.state.Stdin, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	go func() {
		io.Copy(i.Stdin, f)
		i.Stdin.Close()
	}()

	return nil
}

func (p *process) killAll() error {
	return nil
}
