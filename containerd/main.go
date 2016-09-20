package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/pprof"
	"github.com/docker/containerd/supervisor"
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
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{TimestampFormat: time.RFC3339Nano})
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = getVersion()
	app.Usage = usage
	app.Flags = daemonFlags
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		if p := context.GlobalString("pprof-address"); len(p) > 0 {
			h := pprof.New()
			http.Handle("/debug", h)
			go http.ListenAndServe(p, nil)
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
	signals := make(chan os.Signal, 2048)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1)
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
	var (
		listenSpec  = context.String("listen")
		listenParts = strings.SplitN(listenSpec, "://", 2)
	)
	if len(listenParts) != 2 {
		return fmt.Errorf("bad listen address format %s, expected proto://address", listenSpec)
	}
	server, err := startServer(listenParts[0], listenParts[1], sv)
	if err != nil {
		return err
	}
	for s := range signals {
		switch s {
		case syscall.SIGUSR1:
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
		case syscall.SIGINT, syscall.SIGTERM:
			logrus.Infof("stopping containerd after receiving %s", s)
			server.Stop()
			os.Exit(0)
		}
	}
	return nil
}
