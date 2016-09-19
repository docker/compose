package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/api/http/pprof"
	"github.com/docker/containerd/supervisor"
	"github.com/docker/docker/pkg/listeners"
	"github.com/rcrowley/go-metrics"
)

const (
	usage               = `High performance container daemon`
	minRlimit           = 1024
	defaultStateDir     = "/run/containerd"
	defaultGRPCEndpoint = "unix:///run/containerd/containerd.sock"
)

var daemonFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "debug",
		Usage: "enable debug output in the logs",
	},
	cli.StringFlag{
		Name:  "state-dir",
		Value: defaultStateDir,
		Usage: "runtime state directory",
	},
	cli.DurationFlag{
		Name:  "metrics-interval",
		Value: 5 * time.Minute,
		Usage: "interval for flushing metrics to the store",
	},
	cli.StringFlag{
		Name:  "listen,l",
		Value: defaultGRPCEndpoint,
		Usage: "proto://address on which the GRPC API will listen",
	},
	cli.StringFlag{
		Name:  "runtime,r",
		Value: "runc",
		Usage: "name or path of the OCI compliant runtime to use when executing containers",
	},
	cli.StringSliceFlag{
		Name:  "runtime-args",
		Value: &cli.StringSlice{},
		Usage: "specify additional runtime args",
	},
	cli.StringFlag{
		Name:  "shim",
		Value: "containerd-shim",
		Usage: "Name or path of shim",
	},
	cli.StringFlag{
		Name:  "pprof-address",
		Usage: "http address to listen for pprof events",
	},
	cli.DurationFlag{
		Name:  "start-timeout",
		Value: 15 * time.Second,
		Usage: "timeout duration for waiting on a container to start before it is killed",
	},
	cli.IntFlag{
		Name:  "retain-count",
		Value: 500,
		Usage: "number of past events to keep in the event log",
	},
	cli.StringFlag{
		Name:  "graphite-address",
		Usage: "Address of graphite server",
	},
}

// DumpStacks dumps the runtime stack.
func dumpStacks() {
	var (
		buf       []byte
		stackSize int
	)
	bufferLen := 16384
	for stackSize == len(buf) {
		buf = make([]byte, bufferLen)
		stackSize = runtime.Stack(buf, true)
		bufferLen *= 2
	}
	buf = buf[:stackSize]
	logrus.Infof("=== BEGIN goroutine stack dump ===\n%s\n=== END goroutine stack dump ===", buf)
}

func setupDumpStacksTrap() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for range c {
			dumpStacks()
		}
	}()
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{TimestampFormat: time.RFC3339Nano})
	app := cli.NewApp()
	app.Name = "containerd"
	if containerd.GitCommit != "" {
		app.Version = fmt.Sprintf("%s commit: %s", containerd.Version, containerd.GitCommit)
	} else {
		app.Version = containerd.Version
	}
	app.Usage = usage
	app.Flags = daemonFlags
	app.Before = func(context *cli.Context) error {
		setupDumpStacksTrap()
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

	app.Action = func(context *cli.Context) {
		if err := daemon(context); err != nil {
			logrus.Fatal(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func daemon(context *cli.Context) error {
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	sv, err := supervisor.New(
		context.String("state-dir"),
		context.String("runtime"),
		context.String("shim"),
		context.StringSlice("runtime-args"),
		context.Duration("start-timeout"),
		context.Int("retain-count"))
	if err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		w := supervisor.NewWorker(sv, wg)
		go w.Start()
	}
	if err := sv.Start(); err != nil {
		return err
	}
	// Split the listen string of the form proto://addr
	listenSpec := context.String("listen")
	listenParts := strings.SplitN(listenSpec, "://", 2)
	if len(listenParts) != 2 {
		return fmt.Errorf("bad listen address format %s, expected proto://address", listenSpec)
	}
	server, err := startServer(listenParts[0], listenParts[1], sv)
	if err != nil {
		return err
	}
	for ss := range s {
		switch ss {
		default:
			logrus.Infof("stopping containerd after receiving %s", ss)
			server.Stop()
			os.Exit(0)
		}
	}
	return nil
}

func startServer(protocol, address string, sv *supervisor.Supervisor) (*grpc.Server, error) {
	// TODO: We should use TLS.
	// TODO: Add an option for the SocketGroup.
	sockets, err := listeners.Init(protocol, address, "", nil)
	if err != nil {
		return nil, err
	}
	if len(sockets) != 1 {
		return nil, fmt.Errorf("incorrect number of listeners")
	}
	l := sockets[0]
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(sv))
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)

	go func() {
		logrus.Debugf("containerd: grpc api on %s", address)
		if err := s.Serve(l); err != nil {
			logrus.WithField("error", err).Fatal("containerd: serve grpc")
		}
	}()
	return s, nil
}

// getDefaultID returns the hostname for the instance host
func getDefaultID() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
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
