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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cnabio/cnab-to-oci/remotes"
	"github.com/distribution/distribution/v3/reference"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/config"
)

type convertOptions struct {
	*projectOptions
	Format  string
	Output  string
	quiet   bool
	resolve bool
}

var addFlagsFuncs []func(cmd *cobra.Command, opts *convertOptions)

func convertCommand(p *projectOptions) *cobra.Command {
	opts := convertOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Aliases: []string{"config"},
		Use:     "convert SERVICES",
		Short:   "Converts the compose file to platform's canonical format",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.quiet {
				devnull, err := os.Open(os.DevNull)
				if err != nil {
					return err
				}
				os.Stdout = devnull
			}
			return runConvert(cmd.Context(), opts, args)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolve, "resolve-image-digests", false, "Pin image tags to digests.")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything.")

	// add flags for hidden backends
	for _, f := range addFlagsFuncs {
		f(cmd, &opts)
	}
	return cmd
}

func runConvert(ctx context.Context, opts convertOptions, services []string) error {
	var json []byte
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	if opts.resolve {
		configFile, err := cliconfig.Load(config.Dir())
		if err != nil {
			return err
		}

		resolver := remotes.CreateResolver(configFile)
		err = project.ResolveImages(func(named reference.Named) (digest.Digest, error) {
			_, desc, err := resolver.Resolve(ctx, named.String())
			return desc.Digest, err
		})
		if err != nil {
			return err
		}
	}

	json, err = c.ComposeService().Convert(ctx, project, compose.ConvertOptions{
		Format: opts.Format,
		Output: opts.Output,
	})
	if err != nil {
		return err
	}

	if opts.quiet {
		return nil
	}

	var out io.Writer = os.Stdout
	if opts.Output != "" && len(json) > 0 {
		file, err := os.Create(opts.Output)
		if err != nil {
			return err
		}
		out = bufio.NewWriter(file)
	}
	_, err = fmt.Fprint(out, string(json))
	return err
}
