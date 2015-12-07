// +build linux

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd"
	"github.com/docker/containerd/util"
	"github.com/opencontainers/runc/libcontainer/utils"
)

func startSignalHandler(supervisor *containerd.Supervisor, bufferSize int) {
	logrus.Debug("containerd: starting signal handler")
	signals := make(chan os.Signal, bufferSize)
	signal.Notify(signals)
	for s := range signals {
		switch s {
		case syscall.SIGTERM, syscall.SIGINT:
			supervisor.Stop(signals)
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
	supervisor.Close()
	os.Exit(0)
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
	return util.SetSubreaper(1)
}
