package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/docker/containerd/sys"
	"github.com/docker/docker/pkg/term"
)

var logFile *os.File

func writeMessage(f *os.File, level string, err error) {
	fmt.Fprintf(f, `{"level": "%s","msg": "%s"}`, level, err)
	f.Sync()
}

type controlMessage struct {
	Type   int
	Width  int
	Height int
}

// containerd-shim is a small shim that sits in front of a runtime implementation
// that allows it to be repartented to init and handle reattach from the caller.
//
// the cwd of the shim should be the path to the state directory where the shim
// can locate fifos and other information.
// Arg0: id of the container
// Arg1: bundle path
// Arg2: runtime binary
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
	if err := sys.SetSubreaper(1); err != nil {
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
	if err := p.create(); err != nil {
		p.delete()
		return err
	}
	msgC := make(chan controlMessage, 32)
	go func() {
		for {
			var m controlMessage
			if _, err := fmt.Fscanf(control, "%d %d %d\n", &m.Type, &m.Width, &m.Height); err != nil {
				continue
			}
			msgC <- m
		}
	}()
	if runtime.GOOS == "solaris" {
		return nil
	}
	var exitShim bool
	for {
		select {
		case s := <-signals:
			switch s {
			case syscall.SIGCHLD:
				exits, _ := Reap(false)
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
				// kill all processes in the container incase it was not running in
				// its own PID namespace
				p.killAll()
				// wait for all the processes and IO to finish
				p.Wait()
				// delete the container from the runtime
				p.delete()
				// the close of the exit fifo will happen when the shim exits
				return nil
			}
		case msg := <-msgC:
			switch msg.Type {
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
					Width:  uint16(msg.Width),
					Height: uint16(msg.Height),
				}
				term.SetWinsize(p.console.Fd(), &ws)
			}
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

// Exit is the wait4 information from an exited process
type Exit struct {
	Pid    int
	Status int
}

// Reap reaps all child processes for the calling process and returns their
// exit information
func Reap(wait bool) (exits []Exit, err error) {
	var (
		ws  syscall.WaitStatus
		rus syscall.Rusage
	)
	flag := syscall.WNOHANG
	if wait {
		flag = 0
	}
	for {
		pid, err := syscall.Wait4(-1, &ws, flag, &rus)
		if err != nil {
			if err == syscall.ECHILD {
				return exits, nil
			}
			return exits, err
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, Exit{
			Pid:    pid,
			Status: exitStatus(ws),
		})
	}
}

const exitSignalOffset = 128

// exitStatus returns the correct exit status for a process based on if it
// was signaled or exited cleanly
func exitStatus(status syscall.WaitStatus) int {
	if status.Signaled() {
		return exitSignalOffset + int(status.Signal())
	}
	return status.ExitStatus()
}
