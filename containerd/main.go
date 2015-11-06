package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

const (
	Version = "0.0.1"
	Usage   = `High performance conatiner daemon`
)

func main() {
	app := cli.NewApp()
	app.Name = "containerd"
	app.Version = Version
	app.Usage = Usage
	app.Authors = []cli.Author{
		{
			Name:  "@crosbymichael",
			Email: "crosbymichael@gmail.com",
		},
	}
	app.Commands = []cli.Command{
		DaemonCommand,
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "debug", Usage: "enable debug output in the logs"},
		//		cli.StringFlag{Name: "metrics", Value: "stdout", Usage: "metrics file"},
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
