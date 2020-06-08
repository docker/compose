package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/console"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
)

type execOpts struct {
	Tty bool
}

// ExecCommand runs a command in a running container
func ExecCommand() *cobra.Command {
	var opts execOpts
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Run a command in a running container",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExec(cmd.Context(), opts, args[0], strings.Join(args[1:], " "))
		},
	}

	cmd.Flags().BoolVarP(&opts.Tty, "tty", "t", false, "Allocate a pseudo-TTY")

	return cmd
}

func runExec(ctx context.Context, opts execOpts, name string, command string) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	con := console.Current()

	if opts.Tty {
		if err := con.SetRaw(); err != nil {
			return err
		}
		defer func() {
			if err := con.Reset(); err != nil {
				fmt.Println("Unable to close the console")
			}
		}()
	}

	return c.ContainerService().Exec(ctx, name, command, con, con)
}
