package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/osutils"
	"github.com/docker/containerd/supervisor"
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
		Usage: "Address on which GRPC API will listen",
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
	appendPlatformFlags()
	app := cli.NewApp()
	app.Name = "containerd"
	if containerd.GitCommit != "" {
		app.Version = fmt.Sprintf("%s commit: %s", containerd.Version, containerd.GitCommit)
	} else {
		app.Version = containerd.Version
	}
	app.Usage = usage
	app.Flags = daemonFlags
	setAppBefore(app)

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
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)
	if err := osutils.SetSubreaper(1); err != nil {
		logrus.WithField("error", err).Error("containerd: set subpreaper")
	}
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
	server, err := startServer(context.String("listen"), sv)
	if err != nil {
		return err
	}
	for ss := range s {
		switch ss {
		case syscall.SIGCHLD:
			if _, err := osutils.Reap(); err != nil {
				logrus.WithField("error", err).Warn("containerd: reap child processes")
			}
		default:
			logrus.Infof("stopping containerd after receiving %s", ss)
			server.Stop()
			os.Exit(0)
		}
	}
	return nil
}

func startServer(address string, sv *supervisor.Supervisor) (*grpc.Server, error) {
	if err := os.RemoveAll(address); err != nil {
		return nil, err
	}
	l, err := net.Listen(defaultListenType, address)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(sv))
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
