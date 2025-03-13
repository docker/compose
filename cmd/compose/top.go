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

type (
	topHeader  map[string]int // maps a proc title to its output index
	topEntries map[string]string
)

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

	header, entries := collectTop(containers)
	return topPrint(dockerCli.Out(), header, entries)
}

func collectTop(containers []api.ContainerProcSummary) (topHeader, []topEntries) {
	// map column name to its header (should keep working if backend.Top returns
	// varying columns for different containers)
	header := topHeader{"SERVICE": 0, "#": 1}

	// assume one process per container and grow if needed
	entries := make([]topEntries, 0, len(containers))

	for _, container := range containers {
		for _, proc := range container.Processes {
			entry := topEntries{
				"SERVICE": container.Service,
				"#":       container.Replica,
			}
			for i, title := range container.Titles {
				if _, exists := header[title]; !exists {
					header[title] = len(header)
				}
				entry[title] = proc[i]
			}
			entries = append(entries, entry)
		}
	}

	// ensure CMD is the right-most column
	if pos, ok := header["CMD"]; ok {
		maxPos := pos
		for h, i := range header {
			if i > maxPos {
				maxPos = i
			}
			if i > pos {
				header[h] = i - 1
			}
		}
		header["CMD"] = maxPos
	}

	return header, entries
}

func topPrint(out io.Writer, headers topHeader, rows []topEntries) error {
	if len(rows) == 0 {
		return nil
	}

	w := tabwriter.NewWriter(out, 4, 1, 2, ' ', 0)

	// write headers in the order we've encountered them
	h := make([]string, len(headers))
	for title, index := range headers {
		h[index] = title
	}
	_, _ = fmt.Fprintln(w, strings.Join(h, "\t"))

	for _, row := range rows {
		// write proc data in header order
		r := make([]string, len(headers))
		for title, index := range headers {
			if v, ok := row[title]; ok {
				r[index] = v
			} else {
				r[index] = "-"
			}
		}
		_, _ = fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	return w.Flush()
}
