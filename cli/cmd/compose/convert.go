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

	"github.com/docker/compose-cli/api/compose"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
)

type convertOptions struct {
	*projectOptions
	Format string
	Output string
}

var addFlagsFuncs []func(cmd *cobra.Command, opts *convertOptions)

func convertCommand(p *projectOptions) *cobra.Command {
	opts := convertOptions{
		projectOptions: p,
	}
	convertCmd := &cobra.Command{
		Aliases: []string{"config"},
		Use:     "convert SERVICES",
		Short:   "Converts the compose file to platform's canonical format",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd.Context(), opts, args)
		},
	}
	flags := convertCmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")

	// add flags for hidden backends
	for _, f := range addFlagsFuncs {
		f(convertCmd, &opts)
	}
	return convertCmd
}

func runConvert(ctx context.Context, opts convertOptions, services []string) error {
	var json []byte
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	json, err = c.ComposeService().Convert(ctx, project, compose.ConvertOptions{
		Format: opts.Format,
		Output: opts.Output,
	})
	if err != nil {
		return err
	}
	if opts.Output != "" {
		fmt.Print("model saved to ")
	}
	fmt.Println(string(json))
	return nil
}
