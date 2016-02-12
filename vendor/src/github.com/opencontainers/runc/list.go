// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/opencontainers/runc/libcontainer"
)

var listCommand = cli.Command{
	Name:  "list",
	Usage: "lists containers started by runc with the given root",
	Action: func(context *cli.Context) {
		factory, err := loadFactory(context)
		if err != nil {
			logrus.Fatal(err)
		}
		// get the list of containers
		root := context.GlobalString("root")
		absRoot, err := filepath.Abs(root)
		if err != nil {
			logrus.Fatal(err)
		}
		list, err := ioutil.ReadDir(absRoot)
		if err != nil {
			logrus.Fatal(err)
		}
		w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
		fmt.Fprint(w, "ID\tPID\tSTATUS\tCREATED\n")
		// output containers
		for _, item := range list {
			if item.IsDir() {
				if err := outputListInfo(item.Name(), factory, w); err != nil {
					logrus.Fatal(err)
				}
			}
		}
		if err := w.Flush(); err != nil {
			logrus.Fatal(err)
		}
	},
}

func outputListInfo(id string, factory libcontainer.Factory, w *tabwriter.Writer) error {
	container, err := factory.Load(id)
	if err != nil {
		return err
	}
	containerStatus, err := container.Status()
	if err != nil {
		return err
	}
	state, err := container.State()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
		container.ID(),
		state.BaseState.InitProcessPid,
		containerStatus.String(),
		state.BaseState.Created.Format(time.RFC3339Nano))
	return nil
}
