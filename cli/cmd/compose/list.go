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
	"strings"

	"github.com/docker/cli/opts"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/formatter"
)

type lsOptions struct {
	Format string
	Quiet  bool
	All    bool
	Filter opts.FilterOpt
}

func listCommand(contextType string, backend compose.Service) *cobra.Command {
	opts := lsOptions{Filter: opts.NewFilterOpt()}
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List running compose projects",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runList(ctx, backend, opts)
		}),
	}
	lsCmd.Flags().StringVar(&opts.Format, "format", "pretty", "Format the output. Values: [pretty | json].")
	lsCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs.")
	lsCmd.Flags().Var(&opts.Filter, "filter", "Filter output based on conditions provided.")
	if contextType == store.DefaultContextType {
		lsCmd.Flags().BoolVarP(&opts.All, "all", "a", false, "Show all stopped Compose projects")
	}

	return lsCmd
}

var acceptedListFilters = map[string]bool{
	"name": true,
}

func runList(ctx context.Context, backend compose.Service, opts lsOptions) error {
	filters := opts.Filter.Value()
	err := filters.Validate(acceptedListFilters)
	if err != nil {
		return err
	}

	stackList, err := backend.List(ctx, compose.ListOptions{All: opts.All})
	if err != nil {
		return err
	}
	if opts.Quiet {
		for _, s := range stackList {
			fmt.Println(s.Name)
		}
		return nil
	}

	if filters.Len() > 0 {
		var filtered []compose.Stack
		for _, s := range stackList {
			if filters.Contains("name") && !filters.Match("name", s.Name) {
				continue
			}
			filtered = append(filtered, s)
		}
		stackList = filtered
	}

	view := viewFromStackList(stackList)
	return formatter.Print(view, opts.Format, os.Stdout, func(w io.Writer) {
		for _, stack := range view {
			_, _ = fmt.Fprintf(w, "%s\t%s\n", stack.Name, stack.Status)
		}
	}, "NAME", "STATUS")
}

type stackView struct {
	Name   string
	Status string
}

func viewFromStackList(stackList []compose.Stack) []stackView {
	retList := make([]stackView, len(stackList))
	for i, s := range stackList {
		retList[i] = stackView{
			Name:   s.Name,
			Status: strings.TrimSpace(fmt.Sprintf("%s %s", s.Status, s.Reason)),
		}
	}
	return retList
}
