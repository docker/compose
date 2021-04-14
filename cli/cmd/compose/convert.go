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
	"strings"

	"github.com/cnabio/cnab-to-oci/remotes"
	"github.com/compose-spec/compose-go/cli"
	"github.com/distribution/distribution/v3/reference"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/config"
	"github.com/docker/compose-cli/utils"
)

type convertOptions struct {
	*projectOptions
	Format        string
	Output        string
	quiet         bool
	resolve       bool
	noInterpolate bool
	services      bool
	volumes       bool
	profiles      bool
	hash          string
}

var addFlagsFuncs []func(cmd *cobra.Command, opts *convertOptions)

func convertCommand(p *projectOptions, backend compose.Service) *cobra.Command {
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
			if opts.services {
				return runServices(opts)
			}
			if opts.volumes {
				return runVolumes(opts)
			}
			if opts.hash != "" {
				return runHash(opts)
			}
			if opts.profiles {
				return runProfiles(opts, args)
			}

			return runConvert(cmd.Context(), backend, opts, args)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolve, "resolve-image-digests", false, "Pin image tags to digests.")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything.")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables.")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.profiles, "profiles", false, "Print the profile names, one per line.")
	flags.StringVar(&opts.hash, "hash", "", "Print the service config hash, one per line.")

	// add flags for hidden backends
	for _, f := range addFlagsFuncs {
		f(cmd, &opts)
	}
	return cmd
}

func runConvert(ctx context.Context, backend compose.Service, opts convertOptions, services []string) error {
	var json []byte
	project, err := opts.toProject(services, cli.WithInterpolation(!opts.noInterpolate))
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

	json, err = backend.Convert(ctx, project, compose.ConvertOptions{
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

func runServices(opts convertOptions) error {
	project, err := opts.toProject(nil)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		fmt.Println(s.Name)
	}
	return nil
}

func runVolumes(opts convertOptions) error {
	project, err := opts.toProject(nil)
	if err != nil {
		return err
	}
	for n := range project.Volumes {
		fmt.Println(n)
	}
	return nil
}

func runHash(opts convertOptions) error {
	var services []string
	if opts.hash != "*" {
		services = append(services, strings.Split(opts.hash, ",")...)
	}
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		hash, err := utils.ServiceHash(s)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", s.Name, hash)
	}
	return nil
}

func runProfiles(opts convertOptions, services []string) error {
	profiles := map[string]interface{}{}
	_, err := opts.toProject(services)
	if err != nil {
		return err
	}
	if opts.projectOptions != nil {
		for _, p := range opts.projectOptions.Profiles {
			profiles[p] = nil
		}
		for p := range profiles {
			fmt.Println(p)
		}
	}
	return nil
}
