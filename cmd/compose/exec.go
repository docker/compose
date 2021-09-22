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
	"github.com/docker/cli/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type execOpts struct {
	*composeOptions

	service     string
	command     []string
	environment []string
	workingDir  string

	noTty      bool
	user       string
	detach     bool
	index      int
	privileged bool
}

func execCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := execOpts{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	runCmd := &cobra.Command{
		Use:   "exec [options] [-e KEY=VAL...] [--] SERVICE COMMAND [ARGS...]",
		Short: "Execute a command in a running container.",
		Args:  cobra.MinimumNArgs(2),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			opts.service = args[0]
			opts.command = args[1:]
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runExec(ctx, backend, opts)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}

	runCmd.Flags().BoolVarP(&opts.detach, "detach", "d", false, "Detached mode: Run command in the background.")
	runCmd.Flags().StringArrayVarP(&opts.environment, "env", "e", []string{}, "Set environment variables")
	runCmd.Flags().IntVar(&opts.index, "index", 1, "index of the container if there are multiple instances of a service [default: 1].")
	runCmd.Flags().BoolVarP(&opts.privileged, "privileged", "", false, "Give extended privileges to the process.")
	runCmd.Flags().StringVarP(&opts.user, "user", "u", "", "Run the command as this user.")
	runCmd.Flags().BoolVarP(&opts.noTty, "no-TTY", "T", notAtTTY(), "Disable pseudo-TTY allocation. By default `docker compose exec` allocates a TTY.")
	runCmd.Flags().StringVarP(&opts.workingDir, "workdir", "w", "", "Path to workdir directory for this command.")

	runCmd.Flags().SetInterspersed(false)
	return runCmd
}

func runExec(ctx context.Context, backend api.Service, opts execOpts) error {
	project, err := opts.toProjectName()
	if err != nil {
		return err
	}

	execOpts := api.RunOptions{
		Service:     opts.service,
		Command:     opts.command,
		Environment: opts.environment,
		Tty:         !opts.noTty,
		User:        opts.user,
		Privileged:  opts.privileged,
		Index:       opts.index,
		Detach:      opts.detach,
		WorkingDir:  opts.workingDir,

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
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

		execOpts.Stdin = con
		execOpts.Stdout = con
		execOpts.Stderr = con
	}
	exitCode, err := backend.Exec(ctx, project, execOpts)
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
}
