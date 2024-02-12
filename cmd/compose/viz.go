/*
   Copyright 2023 Docker Compose CLI authors

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
	"os"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type vizOptions struct {
	*ProjectOptions
	includeNetworks  bool
	includePorts     bool
	includeImageName bool
	indentationStr   string
}

func vizCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := vizOptions{
		ProjectOptions: p,
	}
	var indentationSize int
	var useSpaces bool

	cmd := &cobra.Command{
		Use:   "viz [OPTIONS]",
		Short: "EXPERIMENTAL - Generate a graphviz graph from your compose file",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			var err error
			opts.indentationStr, err = preferredIndentationStr(indentationSize, useSpaces)
			return err
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runViz(ctx, dockerCli, backend, &opts)
		}),
	}

	cmd.Flags().BoolVar(&opts.includePorts, "ports", false, "Include service's exposed ports in output graph")
	cmd.Flags().BoolVar(&opts.includeNetworks, "networks", false, "Include service's attached networks in output graph")
	cmd.Flags().BoolVar(&opts.includeImageName, "image", false, "Include service's image name in output graph")
	cmd.Flags().IntVar(&indentationSize, "indentation-size", 1, "Number of tabs or spaces to use for indentation")
	cmd.Flags().BoolVar(&useSpaces, "spaces", false, "If given, space character ' ' will be used to indent,\notherwise tab character '\\t' will be used")
	return cmd
}

func runViz(ctx context.Context, dockerCli command.Cli, backend api.Service, opts *vizOptions) error {
	_, _ = fmt.Fprintln(os.Stderr, "viz command is EXPERIMENTAL")
	project, _, err := opts.ToProject(ctx, dockerCli, nil)
	if err != nil {
		return err
	}

	// build graph
	graphStr, _ := backend.Viz(ctx, project, api.VizOptions{
		IncludeNetworks:  opts.includeNetworks,
		IncludePorts:     opts.includePorts,
		IncludeImageName: opts.includeImageName,
		Indentation:      opts.indentationStr,
	})

	fmt.Println(graphStr)

	return nil
}

// preferredIndentationStr returns a single string given the indentation preference
func preferredIndentationStr(size int, useSpace bool) (string, error) {
	if size < 0 {
		return "", fmt.Errorf("invalid indentation size: %d", size)
	}

	indentationStr := "\t"
	if useSpace {
		indentationStr = " "
	}
	return strings.Repeat(indentationStr, size), nil
}
