package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/docker/containerd/osutils"
	"github.com/docker/docker/pkg/term"
)

func writeMessage(f *os.File, level string, err error) {
	fmt.Fprintf(f, `{"level": "%s","msg": "%s"}`, level, err)
}

// containerd-shim is a small shim that sits in front of a runtime implementation
// that allows it to be repartented to init and handle reattach from the caller.
//
// the cwd of the shim should be the bundle for the container.  Arg1 should be the path
// to the state directory where the shim can locate fifos and other information.
func main() {
	flag.Parse()
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(filepath.Join(cwd, "shim-log.json"), os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_SYNC, 0666)
	if err != nil {
		panic(err)
	}
	if err := start(f); err != nil {
		// this means that the runtime failed starting the container and will have the
		// proper error messages in the runtime log so we should to treat this as a
		// shim failure because the sim executed properly
		if err == errRuntime {
			f.Close()
			return
		}
		// log the error instead of writing to stderr because the shim will have
		// /dev/null as it's stdio because it is supposed to be reparented to system
		// init and will not have anyone to read from it
		writeMessage(f, "error", err)
		f.Close()
		os.Exit(1)
	}
}

func start(log *os.File) error {
	// start handling signals as soon as possible so that things are properly reaped
	// or if runtime exits before we hit the handler
	signals := make(chan os.Signal, 2048)
	signal.Notify(signals)
	// set the shim as the subreaper for all orphaned processes created by the container
	if err := osutils.SetSubreaper(1); err != nil {
		return err
	}
	// open the exit pipe
	f, err := os.OpenFile("exit", syscall.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	control, err := os.OpenFile("control", syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer control.Close()
	p, err := newProcess(flag.Arg(0), flag.Arg(1), flag.Arg(2))
	if err != nil {
		return err
	}
	defer func() {
		if err := p.Close(); err != nil {
			writeMessage(log, "warn", err)
		}
	}()
	if err := p.start(); err != nil {
		p.delete()
		return err
	}
	go func() {
		for {
			var msg, w, h int
			if _, err := fmt.Fscanf(control, "%d %d %d\n", &msg, &w, &h); err != nil {
				continue
			}
			switch msg {
			case 0:
				// close stdin
				if p.stdinCloser != nil {
					p.stdinCloser.Close()
				}
			case 1:
				if p.console == nil {
					continue
				}
				ws := term.Winsize{
					Width:  uint16(w),
					Height: uint16(h),
				}
				term.SetWinsize(p.console.Fd(), &ws)
			}
		}
	}()
	var exitShim bool
	for s := range signals {
		switch s {
		case syscall.SIGCHLD:
			exits, _ := osutils.Reap()
			for _, e := range exits {
				// check to see if runtime is one of the processes that has exited
				if e.Pid == p.pid() {
					exitShim = true
					writeInt("exitStatus", e.Status)
				}
			}
		}
		// runtime has exited so the shim can also exit
		if exitShim {
			p.delete()
			p.Wait()
			return nil
		}
	}
	return nil
}

func writeInt(path string, i int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d", i)
	return err
}
