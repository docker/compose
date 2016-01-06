package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	netcontext "golang.org/x/net/context"
)

var eventsCommand = cli.Command{
	Name:  "events",
	Usage: "receive events from the containerd daemon",
	Action: func(context *cli.Context) {
		c := getClient(context)
		events, err := c.Events(netcontext.Background(), &types.EventsRequest{})
		if err != nil {
			fatal(err.Error(), 1)
		}
		w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
		fmt.Fprint(w, "TYPE\tID\tPID\tSTATUS\n")
		w.Flush()
		for {
			e, err := events.Recv()
			if err != nil {
				fatal(err.Error(), 1)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", e.Type, e.Id, e.Pid, e.Status)
			w.Flush()
		}
	},
}
