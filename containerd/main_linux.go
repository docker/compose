package main

import (
	"os"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/cloudfoundry/gosigar"
	"github.com/docker/containerd/osutils"
	"github.com/rcrowley/go-metrics"
)

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
