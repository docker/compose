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

package compose

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/console"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
)

type execOpts struct {
	*composeOptions

	service     string
	command     []string
	environment []string
	workingDir  string

	tty        bool
	user       string
	detach     bool
	index      int
	privileged bool
}

func execCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := execOpts{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	runCmd := &cobra.Command{
		Use:   "exec [options] [-e KEY=VAL...] [--] SERVICE COMMAND [ARGS...]",
		Short: "Execute a command in a running container.",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				opts.command = args[1:]
			}
			opts.service = args[0]
			return runExec(cmd.Context(), backend, opts)
		},
	}

	runCmd.Flags().BoolVarP(&opts.detach, "detach", "d", false, "Detached mode: Run command in the background.")
	runCmd.Flags().StringArrayVarP(&opts.environment, "env", "e", []string{}, "Set environment variables")
	runCmd.Flags().IntVar(&opts.index, "index", 1, "index of the container if there are multiple instances of a service [default: 1].")
	runCmd.Flags().BoolVarP(&opts.privileged, "privileged", "", false, "Give extended privileges to the process.")
	runCmd.Flags().StringVarP(&opts.user, "user", "u", "", "Run the command as this user.")
	runCmd.Flags().BoolVarP(&opts.tty, "", "T", false, "Disable pseudo-tty allocation. By default `docker compose exec` allocates a TTY.")
	runCmd.Flags().StringVarP(&opts.workingDir, "workdir", "w", "", "Path to workdir directory for this command.")

	runCmd.Flags().SetInterspersed(false)
	return runCmd
}

func runExec(ctx context.Context, backend compose.Service, opts execOpts) error {
	project, err := opts.toProject(nil)
	if err != nil {
		return err
	}

	execOpts := compose.RunOptions{
		Service:     opts.service,
		Command:     opts.command,
		Environment: opts.environment,
		Tty:         !opts.tty,
		User:        opts.user,
		Privileged:  opts.privileged,
		Index:       opts.index,
		Detach:      opts.detach,
		WorkingDir:  opts.workingDir,

		Writer: os.Stdout,
		Reader: os.Stdin,
	}

	if execOpts.Tty {
		con := console.Current()
		if err := con.SetRaw(); err != nil {
			return err
		}
		defer func() {
			if err := con.Reset(); err != nil {
				fmt.Println("Unable to close the console")
			}
		}()

		execOpts.Writer = con
		execOpts.Reader = con
	}
	return backend.Exec(ctx, project, execOpts)
}
