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
		cli.StringFlag{
			Name:  "id",
			Value: getDefaultID(),
			Usage: "unique containerd id to identify the instance",
		},
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
		cli.IntFlag{
			Name:  "c,concurrency",
			Value: 10,
			Usage: "set the concurrency level for tasks",
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
		if err := daemon(
			context.String("id"),
			context.String("state-dir"),
			context.Int("concurrency"),
			context.Int("buffer-size"),
		); err != nil {
			logrus.Fatal(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func daemon(id, stateDir string, concurrency, bufferSize int) error {
	tasks := make(chan *containerd.StartTask, concurrency*100)
	supervisor, err := containerd.NewSupervisor(id, stateDir, tasks)
	if err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		w := containerd.NewWorker(supervisor, wg)
		go w.Start()
	}
	// only set containerd as the subreaper if it is not an init process
	if pid := os.Getpid(); pid != 1 {
		if err := setSubReaper(); err != nil {
			return err
		}
	}
	// start the signal handler in the background.
	go startSignalHandler(supervisor, bufferSize)
	if err := supervisor.Start(); err != nil {
		return err
	}
	server := v1.NewServer(supervisor)
	return http.ListenAndServe("localhost:8888", server)
}

// getDefaultID returns the hostname for the instance host
func getDefaultID() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}
