package main

import (
	gocontext "context"
	"fmt"

	"github.com/docker/containerd/api/execution"
	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name:  "delete",
	Usage: "delete a process from containerd store",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "pid, p",
			Usage: "new process id",
		},
	},
	Action: func(context *cli.Context) error {
		executionService, err := getExecutionService(context)
		if err != nil {
			return err
		}

		id := context.Args().First()
		if id == "" {
			return fmt.Errorf("container id must be provided")
		}

		pid := context.String("pid")
		if pid != "" {
			_, err = executionService.DeleteProcess(gocontext.Background(), &execution.DeleteProcessRequest{
				ContainerID: id,
				ProcessID:   pid,
			})
			if err != nil {
				return err
			}
		}

		if _, err := executionService.Delete(gocontext.Background(), &execution.DeleteContainerRequest{
			ID: id,
		}); err != nil {
			return err
		}

		return nil
	},
}
