package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/crosbymichael/containerd"
)

var JournalCommand = cli.Command{
	Name:  "journal",
	Usage: "interact with the containerd journal",
	Subcommands: []cli.Command{
		JournalReplyCommand,
	},
}

var JournalReplyCommand = cli.Command{
	Name:  "replay",
	Usage: "replay a journal to get containerd's state syncronized after a crash",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "addr",
			Value: "localhost:8888",
			Usage: "address of the containerd daemon",
		},
	},
	Action: func(context *cli.Context) {
		if err := replay(context.Args().First(), context.String("addr")); err != nil {
			logrus.Fatal(err)
		}
	},
}

func replay(path, addr string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var events []*containerd.Event
	type entry struct {
		Event *containerd.Event `json:"event"`
	}
	for dec.More() {
		var e entry
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		events = append(events, e.Event)
	}
	c := &http.Client{}
	for _, e := range events {
		switch e.Type {
		case containerd.ExitEventType, containerd.DeleteEventType:
			// ignore these types of events
			continue
		}
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		fmt.Printf("sending %q event\n", e.Type)
		r, err := c.Post("http://"+filepath.Join(addr, "event"), "application/json", bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		if r.Body != nil {
			io.Copy(os.Stdout, r.Body)
			r.Body.Close()
		}
	}
	return nil
}
