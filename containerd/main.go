package main

import (
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/util"
	"github.com/rcrowley/go-metrics"
)

const (
	Usage     = `High performance container daemon`
	minRlimit = 1024
)

var authors = []cli.Author{
	{
		Name:  "@crosbymichael",
		Email: "crosbymichael@gmail.com",
	},
}

var daemonFlags = []cli.Flag{
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
		Name:  "c,concurrency",
		Value: 10,
		Usage: "set the concurrency level for tasks",
	},
	cli.DurationFlag{
		Name:  "metrics-interval",
		Value: 60 * time.Second,
		Usage: "interval for flushing metrics to the store",
	},
	cli.StringFlag{
		Name:  "listen,l",
		Value: "/run/containerd/containerd.sock",
		Usage: "Address on which GRPC API will listen",
	},
	cli.BoolFlag{
		Name:  "oom-notify",
		Usage: "enable oom notifications for containers",
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = containerd.Version
	app.Usage = Usage
	app.Authors = authors
	app.Flags = daemonFlags
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
			if err := debugMetrics(context.GlobalDuration("metrics-interval")); err != nil {
				return err
			}
		}
		if err := checkLimits(); err != nil {
			return err
		}
		return nil
	}
	app.Action = func(context *cli.Context) {
		if err := daemon(
			context.String("id"),
			context.String("listen"),
			context.String("state-dir"),
			context.Int("concurrency"),
			context.Bool("oom-notify"),
		); err != nil {
			logrus.Fatal(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func checkLimits() error {
	var l syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &l); err != nil {
		return err
	}
	if l.Cur <= minRlimit {
		logrus.WithFields(logrus.Fields{
			"current": l.Cur,
			"max":     l.Max,
		}).Warn("low RLIMIT_NOFILE changing to max")
		l.Cur = l.Max
		return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &l)
	}
	return nil
}

func debugMetrics(interval time.Duration) error {
	for name, m := range containerd.Metrics() {
		if err := metrics.DefaultRegistry.Register(name, m); err != nil {
			return err
		}
	}
	processMetrics()
	l := log.New(os.Stdout, "[containerd] ", log.LstdFlags)
	go metrics.Log(metrics.DefaultRegistry, interval, l)
	return nil
}

func processMetrics() {
	var (
		g  = metrics.NewGauge()
		fg = metrics.NewGauge()
	)
	metrics.DefaultRegistry.Register("goroutines", g)
	metrics.DefaultRegistry.Register("fds", fg)
	go func() {
		for range time.Tick(30 * time.Second) {
			g.Update(int64(runtime.NumGoroutine()))
			fds, err := util.GetOpenFds(os.Getpid())
			if err != nil {
				logrus.WithField("error", err).Error("get open fd count")
				continue
			}
			fg.Update(int64(fds))
		}
	}()
}

func daemon(id, address, stateDir string, concurrency int, oom bool) error {
	tasks := make(chan *containerd.StartTask, concurrency*100)
	supervisor, err := containerd.NewSupervisor(id, stateDir, tasks, oom)
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
		logrus.WithFields(logrus.Fields{
			"pid": pid,
		}).Debug("containerd is not init, set as subreaper")
		if err := setSubReaper(); err != nil {
			return err
		}
	}
	// start the signal handler in the background.
	go startSignalHandler(supervisor, containerd.DefaultBufferSize)
	if err := supervisor.Start(); err != nil {
		return err
	}
	if err := os.RemoveAll(address); err != nil {
		return err
	}
	l, err := net.Listen("unix", address)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(supervisor))
	logrus.Debugf("GRPC API listen on %s", address)
	return s.Serve(l)
}

// getDefaultID returns the hostname for the instance host
func getDefaultID() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}
