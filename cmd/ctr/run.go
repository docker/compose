package main

import (
	"fmt"

	gocontext "context"

	"github.com/BurntSushi/toml"
	"github.com/docker/containerd/api/execution"
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
	},
	Action: func(context *cli.Context) error {
		var config runConfig
		if _, err := toml.DecodeFile(context.Args().First(), &config); err != nil {
			return err
		}
		id := context.Args().Get(1)
		if id == "" {
			return fmt.Errorf("container id must be provided")
		}
		executionService, err := getExecutionService(context)
		if err != nil {
			return err
		}
		containerService, err := getContainerService(context)
		if err != nil {
			return err
		}
		cr, err := executionService.Create(gocontext.Background(), &execution.CreateContainerRequest{
			ID:         id,
			BundlePath: context.String("bundle"),
		})
		if err != nil {
			return err
		}
		if _, err := containerService.Start(gocontext.Background(), &execution.StartContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}
		// wait for it to die
		if _, err := executionService.Delete(gocontext.Background(), &execution.DeleteContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}
		return nil
	},
}

func getExecutionService(context *cli.Context) (execution.ExecutionServiceClient, error) {
	return nil, nil
}

func getContainerService(context *cli.Context) (execution.ContainerServiceClient, error) {
	return nil, nil
}
