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
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/cli/formatter"
)

type psOptions struct {
	*projectOptions
	Format string
	Quiet  bool
}

func psCommand(p *projectOptions) *cobra.Command {
	opts := psOptions{
		projectOptions: p,
	}
	psCmd := &cobra.Command{
		Use:   "ps",
		Short: "List containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPs(cmd.Context(), opts)
		},
	}
	psCmd.Flags().StringVar(&opts.Format, "format", "pretty", "Format the output. Values: [pretty | json].")
	psCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	return psCmd
}

func runPs(ctx context.Context, opts psOptions) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	containers, err := c.ComposeService().Ps(ctx, projectName)
	if err != nil {
		return err
	}
	if opts.Quiet {
		for _, s := range containers {
			fmt.Println(s.ID)
		}
		return nil
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	return formatter.Print(containers, opts.Format, os.Stdout,
		func(w io.Writer) {
			for _, container := range containers {
				var ports []string
				for _, p := range container.Publishers {
					if p.URL == "" {
						ports = append(ports, fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol))
					} else {
						ports = append(ports, fmt.Sprintf("%s->%d/%s", p.URL, p.TargetPort, p.Protocol))
					}
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", container.Name, container.Service, container.State, container.Health, strings.Join(ports, ", "))
			}
		},
		"NAME", "SERVICE", "STATE", "HEALTH", "PORTS")
}
