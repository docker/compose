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
	"errors"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type copyOptions struct {
	*ProjectOptions

	source      string
	destination string
	index       int
	all         bool
	followLink  bool
	copyUIDGID  bool
}

func copyCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := copyOptions{
		ProjectOptions: p,
	}
	copyCmd := &cobra.Command{
		Use: `cp [OPTIONS] SERVICE:SRC_PATH DEST_PATH|-
	docker compose cp [OPTIONS] SRC_PATH|- SERVICE:DEST_PATH`,
		Short: "Copy files/folders between a service container and the local filesystem",
		Args:  cli.ExactArgs(2),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if args[0] == "" {
				return errors.New("source can not be empty")
			}
			if args[1] == "" {
				return errors.New("destination can not be empty")
			}
			return nil
		}),
		RunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			opts.source = args[0]
			opts.destination = args[1]
			return runCopy(ctx, dockerCli, backend, opts)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	flags := copyCmd.Flags()
	flags.IntVar(&opts.index, "index", 0, "Index of the container if service has multiple replicas")
	flags.BoolVar(&opts.all, "all", false, "Include containers created by the run command")
	flags.BoolVarP(&opts.followLink, "follow-link", "L", false, "Always follow symbol link in SRC_PATH")
	flags.BoolVarP(&opts.copyUIDGID, "archive", "a", false, "Archive mode (copy all uid/gid information)")

	return copyCmd
}

func runCopy(ctx context.Context, dockerCli command.Cli, backend api.Service, opts copyOptions) error {
	name, err := opts.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}

	return backend.Copy(ctx, name, api.CopyOptions{
		Source:      opts.source,
		Destination: opts.destination,
		All:         opts.all,
		Index:       opts.index,
		FollowLink:  opts.followLink,
		CopyUIDGID:  opts.copyUIDGID,
	})
}
