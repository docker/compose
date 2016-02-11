package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/util"
	"github.com/docker/docker/pkg/term"
)

func setupLogger() {
	f, err := os.OpenFile("/tmp/shim.log", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0755)
	if err != nil {
		panic(err)
	}
	logrus.SetOutput(f)
}

// containerd-shim is a small shim that sits in front of a runc implementation
// that allows it to be repartented to init and handle reattach from the caller.
//
// the cwd of the shim should be the bundle for the container.  Arg1 should be the path
// to the state directory where the shim can locate fifos and other information.
func main() {
	flag.Parse()
	// start handling signals as soon as possible so that things are properly reaped
	// or if runc exits before we hit the handler
	signals := make(chan os.Signal, 2048)
	signal.Notify(signals)
	// set the shim as the subreaper for all orphaned processes created by the container
	if err := util.SetSubreaper(1); err != nil {
		logrus.WithField("error", err).Fatal("shim: set as subreaper")
	}
	// open the exit pipe
	f, err := os.OpenFile("exit", syscall.O_WRONLY, 0)
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: open exit pipe")
	}
	defer f.Close()
	control, err := os.OpenFile("control", syscall.O_RDWR, 0)
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: open control pipe")
	}
	defer control.Close()
	p, err := newProcess(flag.Arg(0), flag.Arg(1))
	if err != nil {
		logrus.WithField("error", err).Fatal("shim: create new process")
	}
	if err := p.start(); err != nil {
		p.delete()
		logrus.WithField("error", err).Fatal("shim: start process")
	}
	go func() {
		for {
			var msg, w, h int
			if _, err := fmt.Fscanf(control, "%d %d %d\n", &msg, &w, &h); err != nil {
				logrus.WithField("error", err).Error("shim: reading from control")
			}
			logrus.Info("got control message")
			switch msg {
			case 0:
				// close stdin
				p.shimIO.Stdin.Close()
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
		logrus.WithField("signal", s).Debug("shim: received signal")
		switch s {
		case syscall.SIGCHLD:
			exits, err := util.Reap()
			if err != nil {
				logrus.WithField("error", err).Error("shim: reaping child processes")
			}
			for _, e := range exits {
				// check to see if runc is one of the processes that has exited
				if e.Pid == p.pid() {
					exitShim = true
					logrus.WithFields(logrus.Fields{
						"pid":    e.Pid,
						"status": e.Status,
					}).Info("shim: runc exited")
					if err := writeInt("exitStatus", e.Status); err != nil {
						logrus.WithFields(logrus.Fields{
							"error":  err,
							"status": e.Status,
						}).Error("shim: write exit status")
					}
				}
			}
		}
		// runc has exited so the shim can also exit
		if exitShim {
			if err := p.Close(); err != nil {
				logrus.WithField("error", err).Error("shim: close stdio")
			}
			if err := p.delete(); err != nil {
				logrus.WithField("error", err).Error("shim: delete runc state")
			}
			return
		}
	}
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
