// +build linux

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd"
	"github.com/opencontainers/runc/libcontainer/utils"
)

// http://man7.org/linux/man-pages/man2/prctl.2.html
//
// If arg2 is nonzero, set the "child subreaper" attribute of the
// calling process; if arg2 is zero, unset the attribute.  When a
// process is marked as a child subreaper, all of the children
// that it creates, and their descendants, will be marked as
// having a subreaper.  In effect, a subreaper fulfills the role
// of init(1) for its descendant processes.  Upon termination of
// a process that is orphaned (i.e., its immediate parent has
// already terminated) and marked as having a subreaper, the
// nearest still living ancestor subreaper will receive a SIGCHLD
// signal and be able to wait(2) on the process to discover its
// termination status.
const PR_SET_CHILD_SUBREAPER = 36

func startSignalHandler(supervisor *containerd.Supervisor, bufferSize int) {
	logrus.Debug("containerd: starting signal handler")
	signals := make(chan os.Signal, bufferSize)
	signal.Notify(signals)
	for s := range signals {
		switch s {
		case syscall.SIGTERM, syscall.SIGINT:
			supervisor.Close()
			os.Exit(0)
		case syscall.SIGCHLD:
			exits, err := reap()
			if err != nil {
				logrus.WithField("error", err).Error("containerd: reaping child processes")
			}
			for _, e := range exits {
				supervisor.SendEvent(e)
			}
		}
	}
}

func reap() (exits []*containerd.Event, err error) {
	var (
		ws  syscall.WaitStatus
		rus syscall.Rusage
	)
	for {
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, &rus)
		if err != nil {
			if err == syscall.ECHILD {
				return exits, nil
			}
			return exits, err
		}
		if pid <= 0 {
			return exits, nil
		}
		e := containerd.NewEvent(containerd.ExitEventType)
		e.Pid = pid
		e.Status = utils.ExitStatus(ws)
		exits = append(exits, e)
	}
}

func setSubReaper() error {
	if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, PR_SET_CHILD_SUBREAPER, 1, 0); err != 0 {
		return err
	}
	return nil
}
