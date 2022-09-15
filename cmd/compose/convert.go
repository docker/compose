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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cnabio/cnab-to-oci/remotes"
	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/distribution/v3/reference"
	cliconfig "github.com/docker/cli/cli/config"
	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

type convertOptions struct {
	*projectOptions
	Format              string
	Output              string
	quiet               bool
	resolveImageDigests bool
	noInterpolate       bool
	noNormalize         bool
	services            bool
	volumes             bool
	profiles            bool
	images              bool
	hash                string
}

func convertCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := convertOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Aliases: []string{"config"},
		Use:     "convert [OPTIONS] [SERVICE...]",
		Short:   "Converts the compose file to platform's canonical format",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.quiet {
				devnull, err := os.Open(os.DevNull)
				if err != nil {
					return err
				}
				os.Stdout = devnull
			}
			if p.Compatibility {
				opts.noNormalize = true
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
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
			if opts.images {
				return runConfigImages(opts, args)
			}

			return runConvert(ctx, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests.")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything.")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables.")
	flags.BoolVar(&opts.noNormalize, "no-normalize", false, "Don't normalize compose model.")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.profiles, "profiles", false, "Print the profile names, one per line.")
	flags.BoolVar(&opts.images, "images", false, "Print the image names, one per line.")
	flags.StringVar(&opts.hash, "hash", "", "Print the service config hash, one per line.")
	flags.StringVarP(&opts.Output, "output", "o", "", "Save to file (default to stdout)")

	return cmd
}

func runConvert(ctx context.Context, backend api.Service, opts convertOptions, services []string) error {
	var content []byte
	project, err := opts.toProject(services,
		cli.WithInterpolation(!opts.noInterpolate),
		cli.WithResolvedPaths(true),
		cli.WithNormalization(!opts.noNormalize),
		cli.WithDiscardEnvFile)

	if err != nil {
		return err
	}

	if opts.resolveImageDigests {
		configFile := cliconfig.LoadDefaultConfigFile(os.Stderr)

		resolver := remotes.CreateResolver(configFile)
		err = project.ResolveImages(func(named reference.Named) (digest.Digest, error) {
			_, desc, err := resolver.Resolve(ctx, named.String())
			return desc.Digest, err
		})
		if err != nil {
			return err
		}
	}

	content, err = backend.Convert(ctx, project, api.ConvertOptions{
		Format: opts.Format,
		Output: opts.Output,
	})
	if err != nil {
		return err
	}

	if !opts.noInterpolate {
		content = escapeDollarSign(content)
	}

	if opts.quiet {
		return nil
	}

	var out io.Writer = os.Stdout
	if opts.Output != "" && len(content) > 0 {
		file, err := os.Create(opts.Output)
		if err != nil {
			return err
		}
		out = bufio.NewWriter(file)
	}
	_, err = fmt.Fprint(out, string(content))
	return err
}

func runServices(opts convertOptions) error {
	project, err := opts.toProject(nil)
	if err != nil {
		return err
	}
	return project.WithServices(project.ServiceNames(), func(s types.ServiceConfig) error {
		fmt.Println(s.Name)
		return nil
	})
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
		hash, err := compose.ServiceHash(s)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", s.Name, hash)
	}
	return nil
}

func runProfiles(opts convertOptions, services []string) error {
	set := map[string]struct{}{}
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.AllServices() {
		for _, p := range s.Profiles {
			set[p] = struct{}{}
		}
	}
	profiles := make([]string, 0, len(set))
	for p := range set {
		profiles = append(profiles, p)
	}
	sort.Strings(profiles)
	for _, p := range profiles {
		fmt.Println(p)
	}
	return nil
}

func runConfigImages(opts convertOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		if s.Image != "" {
			fmt.Println(s.Image)
		} else {
			fmt.Printf("%s_%s\n", project.Name, s.Name)
		}
	}
	return nil
}

func escapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}
