package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd"
	api "github.com/docker/containerd/api/execution"
	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/execution/executors/oci"
	// metrics "github.com/docker/go-metrics"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = containerd.Version
	app.Usage = `
                    __        _                     __
  _________  ____  / /_____ _(_)___  ___  _________/ /
 / ___/ __ \/ __ \/ __/ __ ` + "`" + `/ / __ \/ _ \/ ___/ __  /
/ /__/ /_/ / / / / /_/ /_/ / / / / /  __/ /  / /_/ /
\___/\____/_/ /_/\__/\__,_/_/_/ /_/\___/_/   \__,_/

high performance container runtime
`
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in logs",
		},
		cli.StringFlag{
			Name:  "root",
			Usage: "containerd state directory",
			Value: "/run/containerd",
		},
		cli.StringFlag{
			Name:  "runtime",
			Usage: "default runtime for execution",
			Value: "runc",
		},
		cli.StringFlag{
			Name:  "socket, s",
			Usage: "socket path for containerd's GRPC server",
			Value: "/run/containerd/containerd.sock",
		},
		cli.StringFlag{
			Name:  "metrics-address, m",
			Usage: "tcp address to serve metrics on",
			Value: "127.0.0.1:7897",
		},
	}
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Action = func(context *cli.Context) error {
		signals := make(chan os.Signal, 2048)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

		// if address := context.GlobalString("metrics-address"); address != "" {
		// 	go serveMetrics(address)
		// }

		path := context.GlobalString("socket")
		if path == "" {
			return fmt.Errorf("--socket path cannot be empty")
		}
		l, err := createUnixSocket(path)
		if err != nil {
			return err
		}

		var executor execution.Executor
		switch context.GlobalString("runtime") {
		case "runc":
			executor = oci.New(context.GlobalString("root"))
		}

		execService, err := execution.New(executor)
		if err != nil {
			return err
		}

		server := grpc.NewServer()
		api.RegisterExecutionServiceServer(server, execService)
		go serveGRPC(server, l)

		for s := range signals {
			switch s {
			default:
				logrus.WithField("signal", s).Info("containerd: stopping GRPC server")
				server.Stop()
				return nil
			}
		}
		return nil
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "containerd: %s\n", err)
		os.Exit(1)
	}
}

func createUnixSocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0660); err != nil {
		return nil, err
	}
	if err := syscall.Unlink(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return net.Listen("unix", path)
}

// func serveMetrics(address string) {
// 	m := http.NewServeMux()
// 	m.Handle("/metrics", metrics.Handler())
// 	if err := http.ListenAndServe(address, m); err != nil {
// 		logrus.WithError(err).Fatal("containerd: metrics server failure")
// 	}
// }

func serveGRPC(server *grpc.Server, l net.Listener) {
	defer l.Close()
	if err := server.Serve(l); err != nil {
		l.Close()
		logrus.WithError(err).Fatal("containerd: GRPC server failure")
	}
}
