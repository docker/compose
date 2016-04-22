package main

import (
	"log"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/cloudfoundry/gosigar"
	"github.com/codegangsta/cli"
	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/docker/containerd/api/http/pprof"
	"github.com/docker/containerd/osutils"
	"github.com/docker/containerd/supervisor"
	"github.com/rcrowley/go-metrics"
)

const (
	defaultStateDir     = "/run/containerd"
	defaultGRPCEndpoint = "unix:///run/containerd/containerd.sock"
)

func appendPlatformFlags() {
	daemonFlags = append(daemonFlags, cli.StringFlag{
		Name:  "graphite-address",
		Usage: "Address of graphite server",
	})
}

func setAppBefore(app *cli.App) {
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
			if context.GlobalDuration("metrics-interval") > 0 {
				if err := debugMetrics(context.GlobalDuration("metrics-interval"), context.GlobalString("graphite-address")); err != nil {
					return err
				}
			}

		}
		if p := context.GlobalString("pprof-address"); len(p) > 0 {
			pprof.Enable(p)
		}
		if err := checkLimits(); err != nil {
			return err
		}
		return nil
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
		}).Warn("containerd: low RLIMIT_NOFILE changing to max")
		l.Cur = l.Max
		return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &l)
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
		fds, err := osutils.GetOpenFds(os.Getpid())
		if err != nil {
			logrus.WithField("error", err).Error("containerd: get open fd count")
		}
		fg.Update(int64(fds))
		// get the memory used
		m := sigar.ProcMem{}
		if err := m.Get(os.Getpid()); err != nil {
			logrus.WithField("error", err).Error("containerd: get pid memory information")
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
		go graphite.Graphite(metrics.DefaultRegistry, 10e9, "metrics", addr)
	} else {
		l := log.New(os.Stdout, "[containerd] ", log.LstdFlags)
		go metrics.Log(metrics.DefaultRegistry, interval, l)
	}
	return nil
}
