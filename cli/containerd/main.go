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
			Name:  "socket, s",
			Usage: "socket path for containerd's GRPC server",
			Value: "/run/containerd/containerd.sock",
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

		path := context.GlobalString("socket")
		if path == "" {
			return fmt.Errorf("--socket path cannot be empty")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0660); err != nil {
			return err
		}
		if err := syscall.Unlink(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		l, err := net.Listen("unix", path)
		if err != nil {
			return err
		}

		server := grpc.NewServer()
		go serve(server, l)

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

func serve(server *grpc.Server, l net.Listener) {
	defer l.Close()
	if err := server.Serve(l); err != nil {
		l.Close()
		logrus.WithError(err).Fatal("containerd: GRPC server failure")
	}
}
