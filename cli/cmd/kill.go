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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/hashicorp/go-multierror"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/errdefs"
)

type killOpts struct {
	signal string
}

// KillCommand kills containers
func KillCommand() *cobra.Command {
	var opts killOpts
	cmd := &cobra.Command{
		Use:   "kill",
		Short: "Kill one or more running containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(cmd.Context(), args, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opts.signal, "signal", "s", "KILL", "Signal to send to the container")

	return cmd
}

func runKill(ctx context.Context, args []string, opts killOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	var errs *multierror.Error
	for _, id := range args {
		err := c.ContainerService().Kill(ctx, id, opts.signal)
		if err != nil {
			if errdefs.IsNotFoundError(err) {
				errs = multierror.Append(errs, fmt.Errorf("container %s not found", id))
			} else {
				errs = multierror.Append(errs, err)
			}
			continue
		}
		fmt.Println(id)
	}
	if errs != nil {
		errs.ErrorFormat = formatErrors
	}
	return errs.ErrorOrNil()
}
