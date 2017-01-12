// +build !solaris

package main

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/tonistiigi/fifo"
	"golang.org/x/net/context"
)

// setPDeathSig sets the parent death signal to SIGKILL so that if the
// shim dies the container process also dies.
func setPDeathSig() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
}

// openIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func (p *process) openIO() error {
	p.stdio = &stdio{}
	var (
		uid = p.state.RootUID
		gid = p.state.RootGID
	)

	ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)

	stdinCloser, err := fifo.OpenFifo(ctx, p.state.Stdin, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	p.stdinCloser = stdinCloser

	if p.state.Terminal {
		master, console, err := newConsole(uid, gid)
		if err != nil {
			return err
		}
		p.console = master
		p.consolePath = console
		stdin, err := fifo.OpenFifo(ctx, p.state.Stdin, syscall.O_RDONLY, 0)
		if err != nil {
			return err
		}
		go io.Copy(master, stdin)
		stdoutw, err := fifo.OpenFifo(ctx, p.state.Stdout, syscall.O_WRONLY, 0)
		if err != nil {
			return err
		}
		stdoutr, err := fifo.OpenFifo(ctx, p.state.Stdout, syscall.O_RDONLY, 0)
		if err != nil {
			return err
		}
		p.Add(1)
		go func() {
			io.Copy(stdoutw, master)
			master.Close()
			stdoutr.Close()
			stdoutw.Close()
			p.Done()
		}()
		return nil
	}
	i, err := p.initializeIO(uid)
	if err != nil {
		return err
	}
	p.shimIO = i
	// non-tty
	for name, dest := range map[string]func(wc io.WriteCloser, rc io.Closer){
		p.state.Stdout: func(wc io.WriteCloser, rc io.Closer) {
			p.Add(1)
			go func() {
				io.Copy(wc, i.Stdout)
				p.Done()
				wc.Close()
				rc.Close()
			}()
		},
		p.state.Stderr: func(wc io.WriteCloser, rc io.Closer) {
			p.Add(1)
			go func() {
				io.Copy(wc, i.Stderr)
				p.Done()
				wc.Close()
				rc.Close()
			}()
		},
	} {
		fw, err := fifo.OpenFifo(ctx, name, syscall.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("containerd-shim: opening %s failed: %s", name, err)
		}
		fr, err := fifo.OpenFifo(ctx, name, syscall.O_RDONLY, 0)
		if err != nil {
			return fmt.Errorf("containerd-shim: opening %s failed: %s", name, err)
		}
		dest(fw, fr)
	}

	f, err := fifo.OpenFifo(ctx, p.state.Stdin, syscall.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("containerd-shim: opening %s failed: %s", p.state.Stdin, err)
	}
	go func() {
		io.Copy(i.Stdin, f)
		i.Stdin.Close()
		f.Close()
	}()

	return nil
}

func (p *process) killAll() error {
	if !p.state.Exec {
		cmd := exec.Command(p.runtime, append(p.state.RuntimeArgs, "kill", "--all", p.id, "SIGKILL")...)
		cmd.SysProcAttr = setPDeathSig()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %v", out, err)
		}
	}
	return nil
}
