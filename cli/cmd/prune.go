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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/resources"
)

type pruneOpts struct {
	force  bool
	dryRun bool
}

// PruneCommand deletes backend resources
func PruneCommand() *cobra.Command {
	var opts pruneOpts
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "prune existing resources in current context",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrune(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.force, "force", false, "Also prune running containers and Compose applications")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "List resources to be deleted, but do not delete them")

	return cmd
}

func runPrune(ctx context.Context, opts pruneOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	result, err := c.ResourceService().Prune(ctx, resources.PruneRequest{Force: opts.force, DryRun: opts.dryRun})
	if err != nil {
		return err
	}
	deletedResourcesMsg := "Deleted resources:"
	if opts.dryRun {
		deletedResourcesMsg = "Resources that would be deleted:"
	}
	fmt.Println(deletedResourcesMsg)

	for _, id := range result.DeletedIDs {
		fmt.Println(id)
	}
	if result.Summary != "" {
		fmt.Println(result.Summary)
	}
	return err
}
