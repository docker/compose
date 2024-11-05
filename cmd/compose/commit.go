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

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/opts"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type commitOptions struct {
	*ProjectOptions

	service   string
	reference string

	pause   bool
	comment string
	author  string
	changes opts.ListOpts

	index int
}

func commitCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	options := commitOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "commit [OPTIONS] SERVICE [REPOSITORY[:TAG]]",
		Short: "Create a new image from a service container's changes",
		Args:  cobra.RangeArgs(1, 2),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			options.service = args[0]
			if len(args) > 1 {
				options.reference = args[1]
			}

			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runCommit(ctx, dockerCli, backend, options)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	flags := cmd.Flags()
	flags.IntVar(&options.index, "index", 0, "index of the container if service has multiple replicas.")

	flags.BoolVarP(&options.pause, "pause", "p", true, "Pause container during commit")
	flags.StringVarP(&options.comment, "message", "m", "", "Commit message")
	flags.StringVarP(&options.author, "author", "a", "", `Author (e.g., "John Hannibal Smith <hannibal@a-team.com>")`)
	options.changes = opts.NewListOpts(nil)
	flags.VarP(&options.changes, "change", "c", "Apply Dockerfile instruction to the created image")

	return cmd
}

func runCommit(ctx context.Context, dockerCli command.Cli, backend api.Service, options commitOptions) error {
	projectName, err := options.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}

	commitOptions := api.CommitOptions{
		Service:   options.service,
		Reference: options.reference,
		Pause:     options.pause,
		Comment:   options.comment,
		Author:    options.author,
		Changes:   options.changes,
		Index:     options.index,
	}

	return backend.Commit(ctx, projectName, commitOptions)
}
