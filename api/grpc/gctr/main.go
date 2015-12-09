package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
)

const Usage = `High performance conatiner daemon controller`

func main() {
	app := cli.NewApp()
	app.Name = "ctr"
	app.Version = containerd.Version
	app.Usage = Usage
	app.Authors = []cli.Author{
		{
			Name:  "@crosbymichael",
			Email: "crosbymichael@gmail.com",
		},
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in the logs",
		},
		cli.StringFlag{
			Name:  "addr",
			Value: "http://localhost:8888",
			Usage: "address to the containerd api",
		},
	}
	app.Commands = []cli.Command{
		ContainersCommand,
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

func fatal(err string, code int) {
	fmt.Fprintf(os.Stderr, "[ctr] %s", err)
	os.Exit(code)
}
