package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
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
	Action: func(context *cli.Context) error {
		var config runConfig
		if _, err := toml.DecodeFile(context.Args().First(), &config); err != nil {
			return err
		}
		id := context.Args().Get(1)
		if id == "" {
			return fmt.Errorf("containerd must be provided")
		}
		client, err := getClient(context)
		if err != nil {
			return err
		}
		clone, err := client.Images.Clone(config.Image)
		if err != nil {
			return err
		}
		container, err := client.Containers.Create(CreateRequest{
			Id:     id,
			Mounts: clone.Mounts,
			Process: Process{
				Args:   config.Args,
				Env:    config.Env,
				Cwd:    config.Cwd,
				Uid:    config.Uid,
				Gid:    config.Gid,
				Tty:    config.Tty,
				Stdin:  os.Stdin.Name(),
				Stdout: os.Stdout.Name(),
				Stderr: os.Stderr.Name(),
			},
			Owner: "ctr",
		})
		defer client.Containers.Delete(container)
		if err := client.Networks.Attach(config.Network, container); err != nil {
			return err
		}
		if err := client.Containers.Start(container); err != nil {
			return err
		}
		go forwarSignals(client.Containers.SignalProcess, container.Process)
		events, err := client.Containers.Events(container.Id)
		if err != nil {
			return err
		}
		for event := range events {
			if event.Type == "exit" {
				os.Exit(event.Status)
			}
		}
		return nil
	},
}
