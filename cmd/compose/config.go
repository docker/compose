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

type configOptions struct {
	*ProjectOptions
	Format              string
	Output              string
	quiet               bool
	resolveImageDigests bool
	noInterpolate       bool
	noNormalize         bool
	noResolvePath       bool
	services            bool
	volumes             bool
	profiles            bool
	images              bool
	hash                string
	noConsistency       bool
}

func (o *configOptions) ToProject(services []string) (*types.Project, error) {
	return o.ProjectOptions.ToProject(services,
		cli.WithInterpolation(!o.noInterpolate),
		cli.WithResolvedPaths(!o.noResolvePath),
		cli.WithNormalization(!o.noNormalize),
		cli.WithConsistency(!o.noConsistency),
		cli.WithProfiles(o.Profiles),
		cli.WithDiscardEnvFile)
}

func configCommand(p *ProjectOptions, streams api.Streams, backend api.Service) *cobra.Command {
	opts := configOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Aliases: []string{"convert"}, // for backward compatibility with Cloud integrations
		Use:     "config [OPTIONS] [SERVICE...]",
		Short:   "Parse, resolve and render compose file in canonical format",
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

			return runConfig(ctx, streams, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests.")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything.")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables.")
	flags.BoolVar(&opts.noNormalize, "no-normalize", false, "Don't normalize compose model.")
	flags.BoolVar(&opts.noResolvePath, "no-path-resolution", false, "Don't resolve file paths.")
	flags.BoolVar(&opts.noConsistency, "no-consistency", false, "Don't check model consistency - warning: may produce invalid Compose output")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.profiles, "profiles", false, "Print the profile names, one per line.")
	flags.BoolVar(&opts.images, "images", false, "Print the image names, one per line.")
	flags.StringVar(&opts.hash, "hash", "", "Print the service config hash, one per line.")
	flags.StringVarP(&opts.Output, "output", "o", "", "Save to file (default to stdout)")

	return cmd
}

func runConfig(ctx context.Context, streams api.Streams, backend api.Service, opts configOptions, services []string) error {
	var content []byte
	project, err := opts.ToProject(services)
	if err != nil {
		return err
	}

	content, err = backend.Config(ctx, project, api.ConfigOptions{
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

func runServices(streams api.Streams, opts configOptions) error {
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}
	return project.WithServices(project.ServiceNames(), func(s types.ServiceConfig) error {
		fmt.Fprintln(streams.Out(), s.Name)
		return nil
	})
}

func runVolumes(streams api.Streams, opts configOptions) error {
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}
	for n := range project.Volumes {
		fmt.Fprintln(streams.Out(), n)
	}
	return nil
}

func runHash(streams api.Streams, opts configOptions) error {
	var services []string
	if opts.hash != "*" {
		services = append(services, strings.Split(opts.hash, ",")...)
	}
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}

	if len(services) > 0 {
		err = project.ForServices(services, types.IgnoreDependencies)
		if err != nil {
			return err
		}
	}

	sorted := project.Services
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for _, s := range sorted {
		hash, err := compose.ServiceHash(s)
		if err != nil {
			return err
		}
		fmt.Fprintf(streams.Out(), "%s %s\n", s.Name, hash)
	}
	return nil
}

func runProfiles(streams api.Streams, opts configOptions, services []string) error {
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

func runConfigImages(streams api.Streams, opts configOptions, services []string) error {
	project, err := opts.ToProject(services)
	if err != nil {
		return err
	}
	for _, s := range project.Services {
		fmt.Fprintln(streams.Out(), api.GetImageNameOrDefault(s, project.Name))
	}
	return nil
}

func escapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}
