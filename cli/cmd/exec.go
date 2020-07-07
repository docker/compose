/*
   Copyright 2020 Docker, Inc.

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
	cmd.Flags().BoolP("interactive", "i", false, "Keep STDIN open even if not attached")

	return cmd
}

func runExec(ctx context.Context, opts execOpts, name string, command string) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	if opts.Tty {
		con := console.Current()
		if err := con.SetRaw(); err != nil {
			return err
		}
		defer func() {
			if err := con.Reset(); err != nil {
				fmt.Println("Unable to close the console")
			}
		}()
		return c.ContainerService().Exec(ctx, name, command, con, con)
	}
	return c.ContainerService().Exec(ctx, name, command, os.Stdin, os.Stdout)
}
