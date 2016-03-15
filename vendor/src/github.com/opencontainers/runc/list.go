// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"encoding/json"

	"github.com/codegangsta/cli"
)

const formatOptions = `table or json`

// containerState represents the platform agnostic pieces relating to a
// running container's status and state
type containerState struct {
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
}

var listCommand = cli.Command{
	Name:  "list",
	Usage: "lists containers started by runc with the given root",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "format, f",
			Value: "",
			Usage: `select one of: ` + formatOptions + `.

The default format is table.  The following will output the list of containers
in json format:

    # runc list -f json`,
		},
	},
	Action: func(context *cli.Context) {
		s, err := getContainers(context)
		if err != nil {
			fatal(err)
		}

		switch context.String("format") {
		case "", "table":
			w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
			fmt.Fprint(w, "ID\tPID\tSTATUS\tBUNDLE\tCREATED\n")
			for _, item := range s {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
					item.ID,
					item.InitProcessPid,
					item.Status,
					item.Bundle,
					item.Created.Format(time.RFC3339Nano))
			}
			if err := w.Flush(); err != nil {
				fatal(err)
			}
		case "json":
			data, err := json.Marshal(s)
			if err != nil {
				fatal(err)
			}
			os.Stdout.Write(data)

		default:
			fatalf("invalid format option")
		}
	},
}

func getContainers(context *cli.Context) ([]containerState, error) {
	factory, err := loadFactory(context)
	if err != nil {
		return nil, err
	}
	root := context.GlobalString("root")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	list, err := ioutil.ReadDir(absRoot)
	if err != nil {
		fatal(err)
	}

	var s []containerState
	for _, item := range list {
		if item.IsDir() {
			container, err := factory.Load(item.Name())
			if err != nil {
				return nil, err
			}
			containerStatus, err := container.Status()
			if err != nil {
				return nil, err
			}
			state, err := container.State()
			if err != nil {
				return nil, err
			}
			s = append(s, containerState{
				ID:             state.BaseState.ID,
				InitProcessPid: state.BaseState.InitProcessPid,
				Status:         containerStatus.String(),
				Bundle:         searchLabels(state.Config.Labels, "bundle"),
				Created:        state.BaseState.Created})
		}
	}
	return s, nil
}

func searchLabels(labels []string, query string) string {
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			continue
		}
		if parts[0] == query {
			return parts[1]
		}
	}
	return ""
}
