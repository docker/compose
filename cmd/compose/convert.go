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
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

type convertOptions struct {
	*ProjectOptions
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
	noConsistency       bool
}

func convertCommand(p *ProjectOptions, streams api.Streams, backend api.Service) *cobra.Command {
	opts := convertOptions{
		ProjectOptions: p,
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
				return runServices(streams, opts)
			}
			if opts.volumes {
				return runVolumes(streams, opts)
			}
			if opts.hash != "" {
				return runHash(streams, opts)
			}
			if opts.profiles {
				return runProfiles(streams, opts, args)
			}
			if opts.images {
				return runConfigImages(streams, opts, args)
			}

			return runConvert(ctx, streams, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests.")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything.")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables.")
	flags.BoolVar(&opts.noNormalize, "no-normalize", false, "Don't normalize compose model.")
	flags.BoolVar(&opts.noConsistency, "no-consistency", false, "Don't check model consistency - warning: may produce invalid Compose output")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.profiles, "profiles", false, "Print the profile names, one per line.")
	flags.BoolVar(&opts.images, "images", false, "Print the image names, one per line.")
	flags.StringVar(&opts.hash, "hash", "", "Print the service config hash, one per line.")
	flags.StringVarP(&opts.Output, "output", "o", "", "Save to file (default to stdout)")

	return cmd
}

func runConvert(ctx context.Context, streams api.Streams, backend api.Service, opts convertOptions, services []string) error {
	var content []byte
	project, err := opts.ToProject(services,
		cli.WithInterpolation(!opts.noInterpolate),
		cli.WithResolvedPaths(true),
		cli.WithNormalization(!opts.noNormalize),
		cli.WithConsistency(!opts.noConsistency),
		cli.WithDiscardEnvFile)
	if err != nil {
		return err
	}

	content, err = backend.Convert(ctx, project, api.ConvertOptions{
		Format:              opts.Format,
		Output:              opts.Output,
		ResolveImageDigests: opts.resolveImageDigests,
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

	if opts.Output != "" && len(content) > 0 {
		return os.WriteFile(opts.Output, content, 0o666)
	}
	_, err = fmt.Fprint(streams.Out(), string(content))
	return err
}

func runServices(streams api.Streams, opts convertOptions) error {
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}
	return project.WithServices(project.ServiceNames(), func(s types.ServiceConfig) error {
		fmt.Fprintln(streams.Out(), s.Name)
		return nil
	})
}

func runVolumes(streams api.Streams, opts convertOptions) error {
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}
	for n := range project.Volumes {
		fmt.Fprintln(streams.Out(), n)
	}
	return nil
}

func runHash(streams api.Streams, opts convertOptions) error {
	var services []string
	if opts.hash != "*" {
		services = append(services, strings.Split(opts.hash, ",")...)
	}
	project, err := opts.ToProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		hash, err := compose.ServiceHash(s)
		if err != nil {
			return err
		}
		fmt.Fprintf(streams.Out(), "%s %s\n", s.Name, hash)
	}
	return nil
}

func runProfiles(streams api.Streams, opts convertOptions, services []string) error {
	set := map[string]struct{}{}
	project, err := opts.ToProject(services)
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
		fmt.Fprintln(streams.Out(), p)
	}
	return nil
}

func runConfigImages(streams api.Streams, opts convertOptions, services []string) error {
	project, err := opts.ToProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		if s.Image != "" {
			fmt.Fprintln(streams.Out(), s.Image)
		} else {
			fmt.Fprintf(streams.Out(), "%s%s%s\n", project.Name, api.Separator, s.Name)
		}
	}
	return nil
}

func escapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}
