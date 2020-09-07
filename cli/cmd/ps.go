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
	"text/tabwriter"

	"github.com/docker/compose-cli/utils/formatter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	formatter2 "github.com/docker/compose-cli/formatter"
)

type psOpts struct {
	all   bool
	quiet bool
	json  bool
}

func (o psOpts) validate() error {
	if o.quiet && o.json {
		return errors.New(`cannot combine "quiet" and "json" options`)
	}
	return nil
}

// PsCommand lists containers
func PsCommand() *cobra.Command {
	var opts psOpts
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPs(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Only display IDs")
	cmd.Flags().BoolVarP(&opts.all, "all", "a", false, "Show all containers (default shows just running)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Format output as JSON")

	return cmd
}

func runPs(ctx context.Context, opts psOpts) error {
	err := opts.validate()
	if err != nil {
		return err
	}

	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	containers, err := c.ContainerService().List(ctx, opts.all)
	if err != nil {
		return errors.Wrap(err, "fetch containers")
	}

	if opts.quiet {
		for _, c := range containers {
			fmt.Println(c.ID)
		}
		return nil
	}

	if opts.json {
		j, err := formatter2.ToStandardJSON(containers)
		if err != nil {
			return err
		}
		fmt.Println(j)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprintf(w, "CONTAINER ID\tIMAGE\tCOMMAND\tSTATUS\tPORTS\n")
	format := "%s\t%s\t%s\t%s\t%s\n"
	for _, c := range containers {
		fmt.Fprintf(w, format, c.ID, c.Image, c.Command, c.Status, strings.Join(formatter.PortsToStrings(c.Ports), ", "))
	}

	return w.Flush()
}
