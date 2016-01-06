package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/cloudfoundry/gosigar"
	"github.com/codegangsta/cli"
	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/supervisor"
	"github.com/docker/containerd/util"
	"github.com/rcrowley/go-metrics"
)

const (
	usage     = `High performance container daemon`
	minRlimit = 1024
)

var daemonFlags = []cli.Flag{
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
	cli.StringFlag{
		Name:  "graphite-address",
		Usage: "Address of graphite server",
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = containerd.Version
	app.Usage = usage
	app.Flags = daemonFlags
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
			if err := debugMetrics(context.GlobalDuration("metrics-interval"), context.GlobalString("graphite-address")); err != nil {
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

func debugMetrics(interval time.Duration, graphiteAddr string) error {
	for name, m := range supervisor.Metrics() {
		if err := metrics.DefaultRegistry.Register(name, m); err != nil {
			return err
		}
	}
	processMetrics()
	if graphiteAddr != "" {
		addr, err := net.ResolveTCPAddr("tcp", graphiteAddr)
		if err != nil {
			return err
		}
		logrus.Debugf("Sending metrics to Graphite server on %s", graphiteAddr)
		go graphite.Graphite(metrics.DefaultRegistry, 10e9, "metrics", addr)
	} else {
		l := log.New(os.Stdout, "[containerd] ", log.LstdFlags)
		go metrics.Log(metrics.DefaultRegistry, interval, l)
	}
	return nil
}

func processMetrics() {
	var (
		g    = metrics.NewGauge()
		fg   = metrics.NewGauge()
		memg = metrics.NewGauge()
	)
	metrics.DefaultRegistry.Register("goroutines", g)
	metrics.DefaultRegistry.Register("fds", fg)
	metrics.DefaultRegistry.Register("memory-used", memg)
	collect := func() {
		// update number of goroutines
		g.Update(int64(runtime.NumGoroutine()))
		// collect the number of open fds
		fds, err := util.GetOpenFds(os.Getpid())
		if err != nil {
			logrus.WithField("error", err).Error("get open fd count")
		}
		fg.Update(int64(fds))
		// get the memory used
		m := sigar.ProcMem{}
		if err := m.Get(os.Getpid()); err != nil {
			logrus.WithField("error", err).Error("get pid memory information")
		}
		memg.Update(int64(m.Size))
	}
	go func() {
		collect()
		for range time.Tick(30 * time.Second) {
			collect()
		}
	}()
}

func daemon(address, stateDir string, concurrency int, oom bool) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	go reapProcesses()
	tasks := make(chan *supervisor.StartTask, concurrency*100)
	sv, err := supervisor.New(stateDir, tasks, oom)
	if err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		w := supervisor.NewWorker(sv, wg)
		go w.Start()
	}
	if err := sv.Start(); err != nil {
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
	types.RegisterAPIServer(s, server.NewServer(sv))
	logrus.Debugf("GRPC API listen on %s", address)
	return s.Serve(l)
}

func reapProcesses() {
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD)
	if err := util.SetSubreaper(1); err != nil {
		logrus.WithField("error", err).Error("containerd: set subpreaper")
	}
	for range s {
		if _, err := util.Reap(); err != nil {
			logrus.WithField("error", err).Error("containerd: reap child processes")
		}
	}
}

// getDefaultID returns the hostname for the instance host
func getDefaultID() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}
