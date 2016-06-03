package main

import (
	"fmt"
	"os"
	"time"

	netcontext "golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/types"
)

const usage = `High performance container daemon cli`

type exit struct {
	Code int
}

func main() {
	// We want our defer functions to be run when calling fatal()
	defer func() {
		if e := recover(); e != nil {
			if ex, ok := e.(exit); ok == true {
				os.Exit(ex.Code)
			}
			panic(e)
		}
	}()
	app := cli.NewApp()
	app.Name = "ctr"
	if containerd.GitCommit != "" {
		app.Version = fmt.Sprintf("%s commit: %s", containerd.Version, containerd.GitCommit)
	} else {
		app.Version = containerd.Version
	}
	app.Usage = usage
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in the logs",
		},
		cli.StringFlag{
			Name:  "address",
			Value: "unix:///run/containerd/containerd.sock",
			Usage: "proto://address of GRPC API",
		},
		cli.DurationFlag{
			Name:  "conn-timeout",
			Value: 1 * time.Second,
			Usage: "GRPC connection timeout",
		},
	}
	app.Commands = []cli.Command{
		checkpointCommand,
		containersCommand,
		eventsCommand,
		stateCommand,
		versionCommand,
	}
	app.Before = func(context *cli.Context) error {
		if context.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

var versionCommand = cli.Command{
	Name:  "version",
	Usage: "return the daemon version",
	Action: func(context *cli.Context) {
		c := getClient(context)
		resp, err := c.GetServerVersion(netcontext.Background(), &types.GetServerVersionRequest{})
		if err != nil {
			fatal(err.Error(), 1)
		}
		fmt.Printf("daemon version %d.%d.%d commit: %s\n", resp.Major, resp.Minor, resp.Patch, resp.Revision)
	},
}

func fatal(err string, code int) {
	fmt.Fprintf(os.Stderr, "[ctr] %s\n", err)
	panic(exit{code})
}
