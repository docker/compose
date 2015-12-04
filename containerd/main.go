package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/v1"
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
	tasks := make(chan *containerd.StartTask, concurrency*100)
	supervisor, err := containerd.NewSupervisor(stateDir, tasks)
	if err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		w := containerd.NewWorker(supervisor, wg)
		go w.Start()
	}
	if err := setSubReaper(); err != nil {
		return err
	}
	// start the signal handler in the background.
	go startSignalHandler(supervisor, bufferSize)
	if err := supervisor.Start(); err != nil {
		return err
	}
	server := v1.NewServer(supervisor)
	return http.ListenAndServe("localhost:8888", server)
}
