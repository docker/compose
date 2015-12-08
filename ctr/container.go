package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/v1"
)

var ContainersCommand = cli.Command{
	Name:  "containers",
	Usage: "interact with running containers",
	Subcommands: []cli.Command{
		StartCommand,
		ListCommand,
		KillCommand,
	},
	Action: listContainers,
}

var ListCommand = cli.Command{
	Name:   "list",
	Usage:  "list all running containers",
	Action: listContainers,
}

func listContainers(context *cli.Context) {
	c := v1.NewClient(context.GlobalString("addr"))
	containers, err := c.State()
	if err != nil {
		fatal(err.Error(), 1)
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "ID\tPATH\tSTATUS\n")
	for _, c := range containers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.ID, c.BundlePath, c.State.Status)
	}
	if err := w.Flush(); err != nil {
		logrus.Fatal(err)
	}
}

var StartCommand = cli.Command{
	Name:  "start",
	Usage: "start a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the container",
		},
		cli.StringFlag{
			Name:  "checkpoint,c",
			Value: "",
			Usage: "checkpoint to start the container from",
		},
	},
	Action: func(context *cli.Context) {
		path := context.Args().First()
		if path == "" {
			fatal("bundle path cannot be empty", 1)
		}
		id := context.String("id")
		if id == "" {
			fatal("container id cannot be empty", 1)
		}
		c := v1.NewClient(context.GlobalString("addr"))
		if err := c.Start(id, v1.StartOpts{
			Path:       path,
			Checkpoint: context.String("checkpoint"),
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var KillCommand = cli.Command{
	Name:  "kill",
	Usage: "send a signal to a container or it's processes",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "pid,p",
			Usage: "pid of the process to signal within the container",
		},
		cli.IntFlag{
			Name:  "signal,s",
			Value: 15,
			Usage: "signal to send to the container",
		},
	},
	Action: func(context *cli.Context) {
		id := context.Args().First()
		if id == "" {
			fatal("container id cannot be empty", 1)
		}
		c := v1.NewClient(context.GlobalString("addr"))
		if err := c.SignalProcess(id, context.Int("pid"), context.Int("signal")); err != nil {
			fatal(err.Error(), 1)
		}
	},
}
