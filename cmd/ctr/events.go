package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nats-io/go-nats"
	"github.com/urfave/cli"
)

var eventsCommand = cli.Command{
	Name:  "events",
	Usage: "display containerd events",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "subject, s",
			Usage: "subjects filter",
			Value: "containerd.>",
		},
	},
	Action: func(context *cli.Context) error {
		nc, err := nats.Connect(nats.DefaultURL)
		if err != nil {
			return err
		}
		nec, err := nats.NewEncodedConn(nc, nats.JSON_ENCODER)
		if err != nil {
			nc.Close()
			return err
		}
		defer nec.Close()

		evCh := make(chan *nats.Msg, 64)
		sub, err := nec.Subscribe(context.String("subject"), func(e *nats.Msg) {
			evCh <- e
		})
		if err != nil {
			return err
		}
		defer sub.Unsubscribe()

		for {
			e, more := <-evCh
			if !more {
				break
			}

			var prettyJSON bytes.Buffer

			err := json.Indent(&prettyJSON, e.Data, "", "\t")
			if err != nil {
				fmt.Println(string(e.Data))
			} else {
				fmt.Println(prettyJSON.String())
			}
		}

		return nil
	},
}
