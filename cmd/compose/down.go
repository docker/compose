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

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/docker/compose/v5/pkg/utils"
)

type downOptions struct {
	*ProjectOptions
	all           bool
	removeOrphans bool
	timeChanged   bool
	timeout       int
	volumes       bool
	images        string
}

func downCommand(p *ProjectOptions, dockerCli command.Cli, backendOptions *BackendOptions) *cobra.Command {
	opts := downOptions{
		ProjectOptions: p,
	}
	downCmd := &cobra.Command{
		Use:   "down [OPTIONS] [SERVICES]",
		Short: "Stop and remove containers, networks",
		Long: `Stops containers and removes containers, networks, volumes, and images created by up.

By default, the only things removed are:

- Containers for services defined in the Compose file.
- Networks defined in the networks section of the Compose file.
- The default network, if one is used.

Networks and volumes defined as external are never removed.

Anonymous volumes are not removed by default. However, as they don't have a stable name, they are not automatically
mounted by a subsequent up. For data that needs to persist between updates, use explicit paths as bind mounts or
named volumes.

Use --all to remove every resource for the project, including services from inactive profiles and orphan containers.`,
		Example: `docker compose down
docker compose down -v --remove-orphans
docker compose down --all -v`,
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			opts.timeChanged = cmd.Flags().Changed("timeout")
			if opts.images != "" {
				if opts.images != "all" && opts.images != "local" {
					return fmt.Errorf("invalid value for --rmi: %q", opts.images)
				}
			}
			if opts.all && len(args) > 0 {
				return fmt.Errorf("cannot combine --all with service arguments")
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runDown(ctx, dockerCli, backendOptions, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := downCmd.Flags()
	flags.BoolVar(&opts.all, "all", false, "Remove all resources for the project, including inactive profile services and orphan containers")
	removeOrphans := utils.StringToBool(os.Getenv(ComposeRemoveOrphans))
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", removeOrphans, "Remove containers for services not defined in the Compose file")
	flags.IntVarP(&opts.timeout, "timeout", "t", 0, "Specify a shutdown timeout in seconds")
	flags.BoolVarP(&opts.volumes, "volumes", "v", false, `Remove named volumes declared in the "volumes" section of the Compose file and anonymous volumes attached to containers`)
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

func runDown(ctx context.Context, dockerCli command.Cli, backendOptions *BackendOptions, opts downOptions, services []string) error {
	backend, err := compose.NewComposeService(dockerCli, backendOptions.Options...)
	if err != nil {
		return err
	}

	project, name, err := getDownProjectOrName(ctx, dockerCli, backend, opts, services)
	if err != nil {
		return err
	}

	var timeout *time.Duration
	if opts.timeChanged {
		timeoutValue := time.Duration(opts.timeout) * time.Second
		timeout = &timeoutValue
	}

	return backend.Down(ctx, name, api.DownOptions{
		All:           opts.all,
		RemoveOrphans: opts.removeOrphans,
		Project:       project,
		Timeout:       timeout,
		Images:        opts.images,
		Volumes:       opts.volumes,
		Services:      services,
	})
}

func getDownProjectOrName(ctx context.Context, dockerCli command.Cli, backend api.Compose, opts downOptions, services []string) (*types.Project, string, error) {
	if !opts.all {
		return opts.projectOrName(ctx, dockerCli, services...)
	}

	allProjectOpts := *opts.ProjectOptions
	allProjectOpts.Profiles = []string{"*"}
	allProjectOpts.All = true

	project, _, err := allProjectOpts.ToProject(ctx, dockerCli, backend, nil, composecli.WithDiscardEnvFile, composecli.WithoutEnvironmentResolution)
	if err == nil {
		return project, project.Name, nil
	}

	if len(allProjectOpts.ConfigPaths) > 0 {
		return nil, "", err
	}

	name := allProjectOpts.ProjectName
	if name == "" {
		name = os.Getenv(ComposeProjectName)
	}
	if name != "" {
		return nil, name, nil
	}

	return nil, "", err
}
