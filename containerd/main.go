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
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/v1"
	"github.com/opencontainers/runc/libcontainer/utils"
	"github.com/rcrowley/go-metrics"
)

const Usage = `High performance conatiner daemon`

func main() {
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = containerd.Version
	app.Usage = Usage
	app.Authors = []cli.Author{
		{
			Name:  "@crosbymichael",
			Email: "crosbymichael@gmail.com",
		},
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in the logs",
		},
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
	}
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
			l := log.New(os.Stdout, "[containerd] ", log.LstdFlags)
			goRoutineCounter := metrics.NewGauge()
			metrics.DefaultRegistry.Register("goroutines", goRoutineCounter)
			for name, m := range containerd.Metrics() {
				if err := metrics.DefaultRegistry.Register(name, m); err != nil {
					return err
				}
			}
			go func() {
				for range time.Tick(30 * time.Second) {
					goRoutineCounter.Update(int64(runtime.NumGoroutine()))
				}
			}()
			go metrics.Log(metrics.DefaultRegistry, 60*time.Second, l)
		}
		return nil
	}
	app.Action = func(context *cli.Context) {
		if err := daemon(context.String("state-dir"), 10, context.Int("buffer-size")); err != nil {
			logrus.Fatal(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func daemon(stateDir string, concurrency, bufferSize int) error {
	supervisor, err := containerd.NewSupervisor(stateDir, concurrency)
	if err != nil {
		return err
	}
	events := make(chan *containerd.Event, bufferSize)
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
