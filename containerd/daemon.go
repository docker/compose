package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/crosbymichael/containerd"
	"github.com/opencontainers/runc/libcontainer/utils"
)

var DaemonCommand = cli.Command{
	Name: "daemon",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "state-dir",
			Value: "/run/containerd",
			Usage: "runtime state directory",
		},
		cli.IntFlag{
			Name:  "buffer-size",
			Value: 2048,
			Usage: "set the channel buffer size for events and signals",
		},
	},
	Action: func(context *cli.Context) {
	},
}

func daemon(stateDir string, bufferSize int) error {
	supervisor, err := container.NewSupervisor(stateDir)
	if err != nil {
		return err
	}
	events := make(chan containerd.Event, bufferSize)
	// start the signal handler in the background.
	go startSignalHandler(supervisor, bufferSize)
	return supervisor.Run(events)
}

func startSignalHandler(supervisor *containerd.Supervisor, bufferSize int) {
	logrus.Debug("containerd: starting signal handler")
	signals := make(chan os.Signal, bufferSize)
	signal.Notify(signals)
	for s := range signals {
		logrus.WithField("signal", s).Debug("containerd: received signal")
		switch s {
		case syscall.SIGTERM, syscall.SIGINT, syscall.SIGSTOP:
			supervisor.Stop()
		case syscall.SIGCHLD:
			exits, err := reap()
			if err != nil {
				logrus.WithField("error", err).Error("containerd: reaping child processes")
			}
			for _, e := range exits {
				if err := supervisor.Process(e); err != nil {
					logrus.WithField("error", err).Error("containerd: processing events")
				}
			}
		}
	}
}

func reap() (exits []*containerd.ExitEvent, err error) {
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
			return nil, err
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, *conatinerd.ExitEvent{
			Pid:    pid,
			Status: utils.ExitStatus(ws),
		})
	}
}
