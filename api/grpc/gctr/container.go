package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"google.golang.org/grpc"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	netcontext "golang.org/x/net/context"
)

// TODO: parse flags and pass opts
func getClient() types.APIClient {
	conn, err := grpc.Dial("localhost:8888", grpc.WithInsecure())
	if err != nil {
		fatal(err.Error(), 1)
	}
	return types.NewAPIClient(conn)
}

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
	cli := getClient()
	resp, err := cli.State(netcontext.Background(), &types.StateRequest{})
	if err != nil {
		fatal(err.Error(), 1)
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "ID\tPATH\tSTATUS\n")
	for _, c := range resp.Containers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Id, c.BundlePath, c.Status)
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
		c := getClient()
		if _, err := c.CreateContainer(netcontext.Background(), &types.CreateContainerRequest{
			Id:         id,
			BundlePath: path,
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
		c := getClient()
		if _, err := c.Signal(netcontext.Background(), &types.SignalRequest{
			Id:     id,
			Pid:    uint32(context.Int("pid")),
			Signal: uint32(context.Int("signal")),
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}
