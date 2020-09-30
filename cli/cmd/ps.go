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
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/containers"
	formatter2 "github.com/docker/compose-cli/formatter"
	"github.com/docker/compose-cli/utils/formatter"
)

type psOpts struct {
	all    bool
	quiet  bool
	json   bool
	format string
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
	cmd.Flags().StringVar(&opts.format, "format", "", "Format the output. Values: [pretty | json | go template]. (Default: pretty)")
	_ = cmd.Flags().MarkHidden("json")

	return cmd
}

func (o psOpts) validate() error {
	if o.quiet && o.json {
		return errors.New(`cannot combine "quiet" and "json" options`)
	}
	return nil
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

	containerList, err := c.ContainerService().List(ctx, opts.all)
	if err != nil {
		return errors.Wrap(err, "fetch containers")
	}

	if opts.quiet {
		for _, c := range containerList {
			fmt.Println(c.ID)
		}
		return nil
	}

	if opts.json {
		opts.format = formatter2.JSON
	}

	return formatter2.Print(containerList, opts.format, os.Stdout, func(w io.Writer) {
		for _, c := range containerList {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Image, c.Command, c.Status,
				strings.Join(formatter.PortsToStrings(c.Ports, fqdn(c)), ", "))
		}
	}, "CONTAINER ID", "IMAGE", "COMMAND", "STATUS", "PORTS")
}

func fqdn(container containers.Container) string {
	fqdn := ""
	if container.Config != nil {
		fqdn = container.Config.FQDN
	}
	return fqdn
}
