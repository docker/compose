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
	"fmt"

	"github.com/docker/compose/v2/cmd/formatter"

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/internal"
)

type versionOptions struct {
	format string
	short  bool
}

func versionCommand() *cobra.Command {
	opts := versionOptions{}
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show the Docker Compose version information",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			runVersion(opts)
			return nil
		},
	}
	// define flags for backward compatibility with com.docker.cli
	flags := cmd.Flags()
	flags.StringVarP(&opts.format, "format", "f", "", "Format the output. Values: [pretty | json]. (Default: pretty)")
	flags.BoolVar(&opts.short, "short", false, "Shows only Compose's version number.")

	return cmd
}

func runVersion(opts versionOptions) {
	if opts.short {
		fmt.Println(internal.Version)
		return
	}
	if opts.format == formatter.JSON {
		fmt.Printf(`{"version":%q}\n`, internal.Version)
		return
	}
	fmt.Println("Docker Compose version", internal.Version)
}
