package main

import (
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
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
		Usage: "name of the OCI compliant runtime to use when executing containers",
	},
}

func main() {
	appendPlatformFlags()
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = containerd.Version
	app.Usage = usage
	app.Flags = daemonFlags
	setAppBefore(app)

	app.Action = func(context *cli.Context) {
		if err := daemon(
			context.String("listen"),
			context.String("state-dir"),
			10,
			context.String("runtime"),
		); err != nil {
			logrus.Fatal(err)
		}
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func daemon(address, stateDir string, concurrency int, runtimeName string) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	go reapProcesses()
	sv, err := supervisor.New(stateDir, runtimeName)
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
	l, err := net.Listen(defaultListenType, address)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(sv))
	logrus.Debugf("containerd: grpc api on %s", address)
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
