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
	"time"

	"github.com/docker/compose/v2/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/compose/v2/pkg/api"
)

type downOptions struct {
	*projectOptions
	removeOrphans bool
	timeChanged   bool
	timeout       int
	volumes       bool
	images        string
}

func downCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := downOptions{
		projectOptions: p,
	}
	downCmd := &cobra.Command{
		Use:   "down [OPTIONS]",
		Short: "Stop and remove containers, networks",
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			opts.timeChanged = cmd.Flags().Changed("timeout")
			if opts.images != "" {
				if opts.images != "all" && opts.images != "local" {
					return fmt.Errorf("invalid value for --rmi: %q", opts.images)
				}
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runDown(ctx, backend, opts)
		}),
		Args:              cobra.NoArgs,
		ValidArgsFunction: noCompletion(),
	}
	flags := downCmd.Flags()
	removeOrphans := utils.StringToBool(os.Getenv("COMPOSE_REMOVE_ORPHANS"))
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", removeOrphans, "Remove containers for services not defined in the Compose file.")
	flags.IntVarP(&opts.timeout, "timeout", "t", 10, "Specify a shutdown timeout in seconds")
	flags.BoolVarP(&opts.volumes, "volumes", "v", false, "Remove named volumes declared in the `volumes` section of the Compose file and anonymous volumes attached to containers.")
	flags.StringVar(&opts.images, "rmi", "", `Remove images used by services. "local" remove only images that don't have a custom tag ("local"|"all")`)
	flags.SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "volume" {
			name = "volumes"
			logrus.Warn("--volume is deprecated, please use --volumes")
		}
		return pflag.NormalizedName(name)
	})
	return downCmd
}

func runDown(ctx context.Context, backend api.Service, opts downOptions) error {
	project, name, err := opts.projectOrName()
	if err != nil {
		return err
	}

	var timeout *time.Duration
	if opts.timeChanged {
		timeoutValue := time.Duration(opts.timeout) * time.Second
		timeout = &timeoutValue
	}
	return backend.Down(ctx, name, api.DownOptions{
		RemoveOrphans: opts.removeOrphans,
		Project:       project,
		Timeout:       timeout,
		Images:        opts.images,
		Volumes:       opts.volumes,
	})
}
