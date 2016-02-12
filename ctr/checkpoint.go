package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	netcontext "golang.org/x/net/context"
)

var checkpointCommand = cli.Command{
	Name:  "checkpoints",
	Usage: "list all checkpoints",
	Subcommands: []cli.Command{
		listCheckpointCommand,
		createCheckpointCommand,
		deleteCheckpointCommand,
	},
	Action: listCheckpoints,
}

var listCheckpointCommand = cli.Command{
	Name:   "list",
	Usage:  "list all checkpoints for a container",
	Action: listCheckpoints,
}

func listCheckpoints(context *cli.Context) {
	var (
		c  = getClient(context)
		id = context.Args().First()
	)
	if id == "" {
		fatal("container id cannot be empty", 1)
	}
	resp, err := c.ListCheckpoint(netcontext.Background(), &types.ListCheckpointRequest{
		Id: id,
	})
	if err != nil {
		fatal(err.Error(), 1)
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "NAME\tTCP\tUNIX SOCKETS\tSHELL\n")
	for _, c := range resp.Checkpoints {
		fmt.Fprintf(w, "%s\t%v\t%v\t%v\n", c.Name, c.Tcp, c.UnixSockets, c.Shell)
	}
	if err := w.Flush(); err != nil {
		fatal(err.Error(), 1)
	}
}

var createCheckpointCommand = cli.Command{
	Name:  "create",
	Usage: "create a new checkpoint for the container",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "tcp",
			Usage: "persist open tcp connections",
		},
		cli.BoolFlag{
			Name:  "unix-sockets",
			Usage: "perist unix sockets",
		},
		cli.BoolFlag{
			Name:  "exit",
			Usage: "exit the container after the checkpoint completes successfully",
		},
		cli.BoolFlag{
			Name:  "shell",
			Usage: "checkpoint shell jobs",
		},
	},
	Action: func(context *cli.Context) {
		var (
			containerID = context.Args().Get(0)
			name        = context.Args().Get(1)
		)
		if containerID == "" {
			fatal("container id at cannot be empty", 1)
		}
		if name == "" {
			fatal("checkpoint name cannot be empty", 1)
		}
		c := getClient(context)
		if _, err := c.CreateCheckpoint(netcontext.Background(), &types.CreateCheckpointRequest{
			Id: containerID,
			Checkpoint: &types.Checkpoint{
				Name:        name,
				Exit:        context.Bool("exit"),
				Tcp:         context.Bool("tcp"),
				Shell:       context.Bool("shell"),
				UnixSockets: context.Bool("unix-sockets"),
			},
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var deleteCheckpointCommand = cli.Command{
	Name:  "delete",
	Usage: "delete a container's checkpoint",
	Action: func(context *cli.Context) {
		var (
			containerID = context.Args().Get(0)
			name        = context.Args().Get(1)
		)
		if containerID == "" {
			fatal("container id at cannot be empty", 1)
		}
		if name == "" {
			fatal("checkpoint name cannot be empty", 1)
		}
		c := getClient(context)
		if _, err := c.DeleteCheckpoint(netcontext.Background(), &types.DeleteCheckpointRequest{
			Id:   containerID,
			Name: name,
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}
