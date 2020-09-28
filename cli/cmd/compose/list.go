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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/errdefs"
	"github.com/docker/compose-cli/formatter"
)

func listCommand() *cobra.Command {
	opts := composeOptions{}
	lsCmd := &cobra.Command{
		Use: "ls",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), opts)
		},
	}
	addComposeCommonFlags(lsCmd.Flags(), &opts)
	return lsCmd
}

func addComposeCommonFlags(f *pflag.FlagSet, opts *composeOptions) {
	f.StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	f.StringVar(&opts.Format, "format", "", "Format the output. Values: [pretty | json]. (Default: pretty)")
}

func runList(ctx context.Context, opts composeOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}
	stackList, err := c.ComposeService().List(ctx, opts.Name)
	if err != nil {
		return err
	}

	return printListFormatted(opts.Format, os.Stdout, stackList)
}

func printListFormatted(format string, out io.Writer, stackList []compose.Stack) error {
	var err error
	switch strings.ToLower(format) {
	case formatter.PRETTY, "":
		err = formatter.PrintPrettySection(out, func(w io.Writer) {
			for _, stack := range stackList {
				fmt.Fprintf(w, "%s\t%s\n", stack.Name, stack.Status)
			}
		}, "NAME", "STATUS")
	case formatter.JSON:
		outJSON, err := formatter.ToStandardJSON(stackList)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, outJSON)
	default:
		err = errors.Wrapf(errdefs.ErrParsingFailed, "format value %q could not be parsed", format)
	}
	return err
}
