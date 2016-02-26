package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/osutils"
)

func checkLimits() error {
	var l syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &l); err != nil {
		return err
	}
	if l.Cur <= minRlimit {
		logrus.WithFields(logrus.Fields{
			"current": l.Cur,
			"max":     l.Max,
		}).Warn("containerd: low RLIMIT_NOFILE changing to max")
		l.Cur = l.Max
		return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &l)
	}
	return nil
}

func reapProcesses() {
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD)
	if err := osutils.SetSubreaper(1); err != nil {
		logrus.WithField("error", err).Error("containerd: set subpreaper")
	}
	for range s {
		if _, err := osutils.Reap(); err != nil {
			logrus.WithField("error", err).Error("containerd: reap child processes")
		}
	}
}
