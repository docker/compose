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

package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/docker/api/client"
)

func psCommand() *cobra.Command {
	opts := composeOptions{}
	psCmd := &cobra.Command{
		Use: "ps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPs(cmd.Context(), opts)
		},
	}
	psCmd.Flags().StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	psCmd.Flags().StringVar(&opts.WorkingDir, "workdir", ".", "Work dir")
	psCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return psCmd
}

func runPs(ctx context.Context, opts composeOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	options, err := opts.toProjectOptions()
	if err != nil {
		return err
	}
	serviceList, err := c.ComposeService().Ps(ctx, options)
	if err != nil {
		return err
	}
	err = printSection(os.Stdout, func(w io.Writer) {
		for _, service := range serviceList {
			fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", service.ID, service.Name, service.Replicas, service.Desired, strings.Join(service.Ports, ", "))
		}
	}, "ID", "NAME", "REPLICAS", "PORTS")
	return err
}

func printSection(out io.Writer, printer func(io.Writer), headers ...string) error {
	w := tabwriter.NewWriter(out, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	printer(w)
	return w.Flush()
}
