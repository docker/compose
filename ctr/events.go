package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/golang/protobuf/ptypes"
	netcontext "golang.org/x/net/context"
)

var eventsCommand = cli.Command{
	Name:  "events",
	Usage: "receive events from the containerd daemon",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "timestamp,t",
			Usage: "get events from a specific time stamp in RFC3339Nano format",
		},
	},
	Action: func(context *cli.Context) {
		var (
			t = time.Time{}
			c = getClient(context)
		)
		if ts := context.String("timestamp"); ts != "" {
			from, err := time.Parse(time.RFC3339Nano, ts)
			if err != nil {
				fatal(err.Error(), 1)
			}
			t = from
		}
		tsp, err := ptypes.TimestampProto(t)
		if err != nil {
			fatal(err.Error(), 1)
		}
		events, err := c.Events(netcontext.Background(), &types.EventsRequest{
			Timestamp: tsp,
		})
		if err != nil {
			fatal(err.Error(), 1)
		}
		w := tabwriter.NewWriter(os.Stdout, 31, 1, 1, ' ', 0)
		fmt.Fprint(w, "TIME\tTYPE\tID\tPID\tSTATUS\n")
		w.Flush()
		for {
			e, err := events.Recv()
			if err != nil {
				fatal(err.Error(), 1)
			}
			t, err := ptypes.Timestamp(e.Timestamp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to convert timestamp")
				t = time.Time{}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", t.Format(time.RFC3339Nano), e.Type, e.Id, e.Pid, e.Status)
			w.Flush()
		}
	},
}
