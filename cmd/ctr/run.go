package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	gocontext "context"

	"github.com/docker/containerd/api/execution"
	execEvents "github.com/docker/containerd/execution"
	"github.com/nats-io/go-nats"
	"github.com/urfave/cli"
)

type runConfig struct {
	Image   string `toml:"image"`
	Process struct {
		Args []string `toml:"args"`
		Env  []string `toml:"env"`
		Cwd  string   `toml:"cwd"`
		Uid  int      `toml:"uid"`
		Gid  int      `toml:"gid"`
		Tty  bool     `toml:"tty"`
	} `toml:"process"`
	Network struct {
		Type    string `toml:"type"`
		IP      string `toml:"ip"`
		Gateway string `toml:"gateway"`
	} `toml:"network"`
}

var runCommand = cli.Command{
	Name:  "run",
	Usage: "run a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Usage: "path to the container's bundle",
		},
		cli.BoolFlag{
			Name:  "tty, t",
			Usage: "allocate a TTY for the container",
		},
	},
	Action: func(context *cli.Context) error {
		// var config runConfig
		// if _, err := toml.DecodeFile(context.Args().First(), &config); err != nil {
		// 	return err
		// }
		id := context.Args().First()
		if id == "" {
			return fmt.Errorf("container id must be provided")
		}
		executionService, err := getExecutionService(context)
		if err != nil {
			return err
		}

		// setup our event subscriber
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

		evCh := make(chan *execEvents.ContainerExitEvent, 64)
		sub, err := nec.Subscribe(execEvents.ContainersEventsSubjectSubscriber, func(e *execEvents.ContainerExitEvent) {
			evCh <- e
		})
		if err != nil {
			return err
		}
		defer sub.Unsubscribe()

		tmpDir, err := getTempDir(id)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		crOpts := &execution.CreateContainerRequest{
			ID:         id,
			BundlePath: context.String("bundle"),
			Console:    context.Bool("tty"),
			Stdin:      filepath.Join(tmpDir, "stdin"),
			Stdout:     filepath.Join(tmpDir, "stdout"),
			Stderr:     filepath.Join(tmpDir, "stderr"),
		}

		fwg, err := prepareStdio(crOpts.Stdin, crOpts.Stdout, crOpts.Stderr)
		if err != nil {
			return err
		}

		cr, err := executionService.Create(gocontext.Background(), crOpts)
		if err != nil {
			return err
		}

		if _, err := executionService.Start(gocontext.Background(), &execution.StartContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}

		var ec uint32
	eventLoop:
		for {
			select {
			case e, more := <-evCh:
				if !more {
					fmt.Println("No More!")
					break eventLoop
				}

				if e.ID == cr.Container.ID && e.PID == cr.InitProcess.ID {
					ec = e.StatusCode
					break eventLoop
				}
			case <-time.After(1 * time.Second):
				if nec.Conn.Status() != nats.CONNECTED {
					break eventLoop
				}
			}
		}

		if _, err := executionService.Delete(gocontext.Background(), &execution.DeleteContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}

		// Ensure we read all io
		fwg.Wait()

		os.Exit(int(ec))

		return nil
	},
}
