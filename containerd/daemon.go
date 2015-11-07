package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/crosbymichael/containerd"
	"github.com/crosbymichael/containerd/api/v1"
	"github.com/opencontainers/runc/libcontainer/utils"
	"github.com/rcrowley/go-metrics"
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
		if context.GlobalBool("debug") {
			l := log.New(os.Stdout, "[containerd] ", log.LstdFlags)
			goRoutineCounter := metrics.NewGauge()
			metrics.DefaultRegistry.Register("goroutines", goRoutineCounter)
			go func() {
				for range time.Tick(30 * time.Second) {
					goRoutineCounter.Update(int64(runtime.NumGoroutine()))
				}
			}()
			go metrics.Log(metrics.DefaultRegistry, 60*time.Second, l)
		}
		if err := daemon(context.String("state-dir"), 10, context.Int("buffer-size")); err != nil {
			logrus.Fatal(err)
		}
	},
}

func daemon(stateDir string, concurrency, bufferSize int) error {
	supervisor, err := containerd.NewSupervisor(stateDir, concurrency)
	if err != nil {
		return err
	}
	events := make(chan containerd.Event, bufferSize)
	// start the signal handler in the background.
	go startSignalHandler(supervisor, bufferSize)
	if err := supervisor.Start(events); err != nil {
		return err
	}
	server := v1.NewServer(supervisor)
	return http.ListenAndServe("localhost:8888", server)
}

func startSignalHandler(supervisor *containerd.Supervisor, bufferSize int) {
	logrus.Debug("containerd: starting signal handler")
	signals := make(chan os.Signal, bufferSize)
	signal.Notify(signals)
	for s := range signals {
		switch s {
		case syscall.SIGTERM, syscall.SIGINT, syscall.SIGSTOP:
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
			return exits, err
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, &containerd.ExitEvent{
			Pid:    pid,
			Status: utils.ExitStatus(ws),
		})
	}
}
