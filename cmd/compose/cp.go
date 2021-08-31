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
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type copyOptions struct {
	*projectOptions

	source      string
	destination string
	index       int
	all         bool
	followLink  bool
	copyUIDGID  bool
}

func copyCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := copyOptions{
		projectOptions: p,
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
		RunE: Adapt(func(ctx context.Context, args []string) error {
			opts.source = args[0]
			opts.destination = args[1]
			return runCopy(ctx, backend, opts)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}

	flags := copyCmd.Flags()
	flags.IntVar(&opts.index, "index", 1, "Index of the container if there are multiple instances of a service [default: 1].")
	flags.BoolVar(&opts.all, "all", false, "Copy to all the containers of the service.")
	flags.BoolVarP(&opts.followLink, "follow-link", "L", false, "Always follow symbol link in SRC_PATH")
	flags.BoolVarP(&opts.copyUIDGID, "archive", "a", false, "Archive mode (copy all uid/gid information)")

	return copyCmd
}

func runCopy(ctx context.Context, backend api.Service, opts copyOptions) error {
	projects, err := opts.toProject(nil)
	if err != nil {
		return err
	}

	return backend.Copy(ctx, projects, api.CopyOptions{
		Source:      opts.source,
		Destination: opts.destination,
		All:         opts.all,
		Index:       opts.index,
		FollowLink:  opts.followLink,
		CopyUIDGID:  opts.copyUIDGID,
	})
}
