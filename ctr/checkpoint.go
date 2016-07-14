package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	netcontext "golang.org/x/net/context"
)

var checkpointSubCmds = []cli.Command{
	listCheckpointCommand,
	createCheckpointCommand,
	deleteCheckpointCommand,
}

var checkpointCommand = cli.Command{
	Name:        "checkpoints",
	Usage:       "list all checkpoints",
	ArgsUsage:   "COMMAND [arguments...]",
	Subcommands: checkpointSubCmds,
	Description: func() string {
		desc := "\n    COMMAND:\n"
		for _, command := range checkpointSubCmds {
			desc += fmt.Sprintf("    %-10.10s%s\n", command.Name, command.Usage)
		}
		return desc
	}(),
	Action: listCheckpoints,
}

var listCheckpointCommand = cli.Command{
	Name:   "list",
	Usage:  "list all checkpoints for a container",
	Action: listCheckpoints,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "checkpoint-dir",
			Value: "",
			Usage: "path to checkpoint directory",
		},
	},
}

func listCheckpoints(context *cli.Context) {
	var (
		c  = getClient(context)
		id = context.Args().First()
	)
	if id == "" {
		fatal("container id cannot be empty", ExitStatusMissingArg)
	}
	resp, err := c.ListCheckpoint(netcontext.Background(), &types.ListCheckpointRequest{
		Id:            id,
		CheckpointDir: context.String("checkpoint-dir"),
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
			Usage: "persist unix sockets",
		},
		cli.BoolFlag{
			Name:  "exit",
			Usage: "exit the container after the checkpoint completes successfully",
		},
		cli.BoolFlag{
			Name:  "shell",
			Usage: "checkpoint shell jobs",
		},
		cli.StringFlag{
			Name:  "checkpoint-dir",
			Value: "",
			Usage: "directory to store checkpoints",
		},
		cli.StringSliceFlag{
			Name:  "empty-ns",
			Usage: "create a namespace, but don't restore its properties",
		},
	},
	Action: func(context *cli.Context) {
		var (
			containerID = context.Args().Get(0)
			name        = context.Args().Get(1)
		)
		if containerID == "" {
			fatal("container id at cannot be empty", ExitStatusMissingArg)
		}
		if name == "" {
			fatal("checkpoint name cannot be empty", ExitStatusMissingArg)
		}
		c := getClient(context)
		checkpoint := types.Checkpoint{
			Name:        name,
			Exit:        context.Bool("exit"),
			Tcp:         context.Bool("tcp"),
			Shell:       context.Bool("shell"),
			UnixSockets: context.Bool("unix-sockets"),
		}

		emptyNSes := context.StringSlice("empty-ns")
		checkpoint.EmptyNS = append(checkpoint.EmptyNS, emptyNSes...)

		if _, err := c.CreateCheckpoint(netcontext.Background(), &types.CreateCheckpointRequest{
			Id:            containerID,
			CheckpointDir: context.String("checkpoint-dir"),
			Checkpoint:    &checkpoint,
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var deleteCheckpointCommand = cli.Command{
	Name:  "delete",
	Usage: "delete a container's checkpoint",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "checkpoint-dir",
			Value: "",
			Usage: "path to checkpoint directory",
		},
	},
	Action: func(context *cli.Context) {
		var (
			containerID = context.Args().Get(0)
			name        = context.Args().Get(1)
		)
		if containerID == "" {
			fatal("container id at cannot be empty", ExitStatusMissingArg)
		}
		if name == "" {
			fatal("checkpoint name cannot be empty", ExitStatusMissingArg)
		}
		c := getClient(context)
		if _, err := c.DeleteCheckpoint(netcontext.Background(), &types.DeleteCheckpointRequest{
			Id:            containerID,
			Name:          name,
			CheckpointDir: context.String("checkpoint-dir"),
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}
