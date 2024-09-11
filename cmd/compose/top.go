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
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type topOptions struct {
	*ProjectOptions
}

func topCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := topOptions{
		ProjectOptions: p,
	}
	topCmd := &cobra.Command{
		Use:   "top [SERVICES...]",
		Short: "Display the running processes",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runTop(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	return topCmd
}

func runTop(ctx context.Context, dockerCli command.Cli, backend api.Service, opts topOptions, services []string) error {
	projectName, err := opts.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}
	containers, err := backend.Top(ctx, projectName, services)
	if err != nil {
		return err
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	for _, container := range containers {
		_, _ = fmt.Fprintf(dockerCli.Out(), "%s\n", container.Name)
		err := psPrinter(dockerCli.Out(), func(w io.Writer) {
			for _, proc := range container.Processes {
				info := []interface{}{}
				for _, p := range proc {
					info = append(info, p)
				}
				_, _ = fmt.Fprintf(w, strings.Repeat("%s\t", len(info))+"\n", info...)

			}
			_, _ = fmt.Fprintln(w)
		},
			container.Titles...)
		if err != nil {
			return err
		}
	}
	return nil
}

func psPrinter(out io.Writer, printer func(writer io.Writer), headers ...string) error {
	w := tabwriter.NewWriter(out, 5, 1, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))
	printer(w)
	return w.Flush()
}
