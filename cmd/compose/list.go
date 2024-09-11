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
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/formatter"

	"github.com/docker/cli/opts"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type lsOptions struct {
	Format string
	Quiet  bool
	All    bool
	Filter opts.FilterOpt
}

func listCommand(dockerCli command.Cli, backend api.Service) *cobra.Command {
	lsOpts := lsOptions{Filter: opts.NewFilterOpt()}
	lsCmd := &cobra.Command{
		Use:   "ls [OPTIONS]",
		Short: "List running compose projects",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runList(ctx, dockerCli, backend, lsOpts)
		}),
		Args:              cobra.NoArgs,
		ValidArgsFunction: noCompletion(),
	}
	lsCmd.Flags().StringVar(&lsOpts.Format, "format", "table", "Format the output. Values: [table | json]")
	lsCmd.Flags().BoolVarP(&lsOpts.Quiet, "quiet", "q", false, "Only display IDs")
	lsCmd.Flags().Var(&lsOpts.Filter, "filter", "Filter output based on conditions provided")
	lsCmd.Flags().BoolVarP(&lsOpts.All, "all", "a", false, "Show all stopped Compose projects")

	return lsCmd
}

var acceptedListFilters = map[string]bool{
	"name": true,
}

func runList(ctx context.Context, dockerCli command.Cli, backend api.Service, lsOpts lsOptions) error {
	filters := lsOpts.Filter.Value()
	err := filters.Validate(acceptedListFilters)
	if err != nil {
		return err
	}

	stackList, err := backend.List(ctx, api.ListOptions{All: lsOpts.All})
	if err != nil {
		return err
	}

	if filters.Len() > 0 {
		var filtered []api.Stack
		for _, s := range stackList {
			if filters.Contains("name") && !filters.Match("name", s.Name) {
				continue
			}
			filtered = append(filtered, s)
		}
		stackList = filtered
	}

	if lsOpts.Quiet {
		for _, s := range stackList {
			_, _ = fmt.Fprintln(dockerCli.Out(), s.Name)
		}
		return nil
	}

	view := viewFromStackList(stackList)
	return formatter.Print(view, lsOpts.Format, dockerCli.Out(), func(w io.Writer) {
		for _, stack := range view {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", stack.Name, stack.Status, stack.ConfigFiles)
		}
	}, "NAME", "STATUS", "CONFIG FILES")
}

type stackView struct {
	Name        string
	Status      string
	ConfigFiles string
}

func viewFromStackList(stackList []api.Stack) []stackView {
	retList := make([]stackView, len(stackList))
	for i, s := range stackList {
		retList[i] = stackView{
			Name:        s.Name,
			Status:      strings.TrimSpace(fmt.Sprintf("%s %s", s.Status, s.Reason)),
			ConfigFiles: s.ConfigFiles,
		}
	}
	return retList
}
