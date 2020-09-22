/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/console"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/containers"
)

type execOpts struct {
	tty         bool
	interactive bool
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

	cmd.Flags().BoolVarP(&opts.tty, "tty", "t", false, "Allocate a pseudo-TTY")
	cmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", false, "Keep STDIN open even if not attached")

	return cmd
}

func runExec(ctx context.Context, opts execOpts, name string, command string) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	request := containers.ExecRequest{
		Command:     command,
		Tty:         opts.tty,
		Interactive: opts.interactive,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}

	if opts.tty {
		con := console.Current()
		if err := con.SetRaw(); err != nil {
			return err
		}
		defer func() {
			if err := con.Reset(); err != nil {
				fmt.Println("Unable to close the console")
			}
		}()

		request.Stdin = con
		request.Stdout = con
		request.Stderr = con
	}

	return c.ContainerService().Exec(ctx, name, request)
}
