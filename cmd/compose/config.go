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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/template"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
	variables           bool
	environment         bool
}

func (o *configOptions) ToProject(ctx context.Context, dockerCli command.Cli, services []string, po ...cli.ProjectOptionsFn) (*types.Project, error) {
	po = append(po, o.ToProjectOptions()...)
	project, _, err := o.ProjectOptions.ToProject(ctx, dockerCli, services, po...)
	return project, err
}

func (o *configOptions) ToModel(ctx context.Context, dockerCli command.Cli, services []string, po ...cli.ProjectOptionsFn) (map[string]any, error) {
	po = append(po, o.ToProjectOptions()...)
	return o.ProjectOptions.ToModel(ctx, dockerCli, services, po...)
}

func (o *configOptions) ToProjectOptions() []cli.ProjectOptionsFn {
	return []cli.ProjectOptionsFn{
		cli.WithInterpolation(!o.noInterpolate),
		cli.WithResolvedPaths(!o.noResolvePath),
		cli.WithNormalization(!o.noNormalize),
		cli.WithConsistency(!o.noConsistency),
		cli.WithDefaultProfiles(o.Profiles...),
		cli.WithDiscardEnvFile,
	}
}

func configCommand(p *ProjectOptions, dockerCli command.Cli) *cobra.Command {
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
				return runServices(ctx, dockerCli, opts)
			}
			if opts.volumes {
				return runVolumes(ctx, dockerCli, opts)
			}
			if opts.hash != "" {
				return runHash(ctx, dockerCli, opts)
			}
			if opts.profiles {
				return runProfiles(ctx, dockerCli, opts, args)
			}
			if opts.images {
				return runConfigImages(ctx, dockerCli, opts, args)
			}
			if opts.variables {
				return runVariables(ctx, dockerCli, opts, args)
			}
			if opts.environment {
				return runEnvironment(ctx, dockerCli, opts, args)
			}

			return runConfig(ctx, dockerCli, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables")
	flags.BoolVar(&opts.noNormalize, "no-normalize", false, "Don't normalize compose model")
	flags.BoolVar(&opts.noResolvePath, "no-path-resolution", false, "Don't resolve file paths")
	flags.BoolVar(&opts.noConsistency, "no-consistency", false, "Don't check model consistency - warning: may produce invalid Compose output")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.profiles, "profiles", false, "Print the profile names, one per line.")
	flags.BoolVar(&opts.images, "images", false, "Print the image names, one per line.")
	flags.StringVar(&opts.hash, "hash", "", "Print the service config hash, one per line.")
	flags.BoolVar(&opts.variables, "variables", false, "Print model variables and default values.")
	flags.BoolVar(&opts.environment, "environment", false, "Print environment used for interpolation.")
	flags.StringVarP(&opts.Output, "output", "o", "", "Save to file (default to stdout)")

	return cmd
}

func runConfig(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) (err error) {
	var content []byte
	if opts.noInterpolate {
		content, err = runConfigNoInterpolate(ctx, dockerCli, opts, services)
		if err != nil {
			return err
		}
	} else {
		content, err = runConfigInterpolate(ctx, dockerCli, opts, services)
		if err != nil {
			return err
		}
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
	_, err = fmt.Fprint(dockerCli.Out(), string(content))
	return err
}

func runConfigInterpolate(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) ([]byte, error) {
	project, err := opts.ToProject(ctx, dockerCli, services)
	if err != nil {
		return nil, err
	}

	if opts.resolveImageDigests {
		project, err = project.WithImagesResolved(compose.ImageDigestResolver(ctx, dockerCli.ConfigFile(), dockerCli.Client()))
		if err != nil {
			return nil, err
		}
	}

	if !opts.noConsistency {
		err := project.CheckContainerNameUnicity()
		if err != nil {
			return nil, err
		}
	}

	var content []byte
	switch opts.Format {
	case "json":
		content, err = project.MarshalJSON()
	case "yaml":
		content, err = project.MarshalYAML()
	default:
		return nil, fmt.Errorf("unsupported format %q", opts.Format)
	}
	if err != nil {
		return nil, err
	}
	return content, nil
}

func runConfigNoInterpolate(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) ([]byte, error) {
	// we can't use ToProject, so the model we render here is only partially resolved
	model, err := opts.ToModel(ctx, dockerCli, services)
	if err != nil {
		return nil, err
	}

	if opts.resolveImageDigests {
		err = resolveImageDigests(ctx, dockerCli, model)
		if err != nil {
			return nil, err
		}
	}

	return formatModel(model, opts.Format)
}

func resolveImageDigests(ctx context.Context, dockerCli command.Cli, model map[string]any) (err error) {
	// create a pseudo-project so we can rely on WithImagesResolved to resolve images
	p := &types.Project{
		Services: types.Services{},
	}
	services := model["services"].(map[string]any)
	for name, s := range services {
		service := s.(map[string]any)
		if image, ok := service["image"]; ok {
			p.Services[name] = types.ServiceConfig{
				Image: image.(string),
			}
		}
	}

	p, err = p.WithImagesResolved(compose.ImageDigestResolver(ctx, dockerCli.ConfigFile(), dockerCli.Client()))
	if err != nil {
		return err
	}

	// Collect image resolved with digest and update model accordingly
	for name, s := range services {
		service := s.(map[string]any)
		config := p.Services[name]
		if config.Image != "" {
			service["image"] = config.Image
		}
		services[name] = service
	}
	model["services"] = services
	return nil
}

func formatModel(model map[string]any, format string) (content []byte, err error) {
	switch format {
	case "json":
		content, err = json.MarshalIndent(model, "", "  ")
	case "yaml":
		buf := bytes.NewBuffer([]byte{})
		encoder := yaml.NewEncoder(buf)
		encoder.SetIndent(2)
		err = encoder.Encode(model)
		content = buf.Bytes()
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	return
}

func runServices(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	project, err := opts.ToProject(ctx, dockerCli, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	err = project.ForEachService(project.ServiceNames(), func(serviceName string, _ *types.ServiceConfig) error {
		fmt.Fprintln(dockerCli.Out(), serviceName)
		return nil
	})
	return err
}

func runVolumes(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	project, err := opts.ToProject(ctx, dockerCli, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	for n := range project.Volumes {
		fmt.Fprintln(dockerCli.Out(), n)
	}
	return nil
}

func runHash(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	var services []string
	if opts.hash != "*" {
		services = append(services, strings.Split(opts.hash, ",")...)
	}
	project, err := opts.ToProject(ctx, dockerCli, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}

	if err := applyPlatforms(project, true); err != nil {
		return err
	}

	if len(services) == 0 {
		services = project.ServiceNames()
	}

	sorted := services
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	for _, name := range sorted {
		s, err := project.GetService(name)
		if err != nil {
			return err
		}

		hash, err := compose.ServiceHash(s)

		if err != nil {
			return err
		}
		fmt.Fprintf(dockerCli.Out(), "%s %s\n", name, hash)
	}
	return nil
}

func runProfiles(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	set := map[string]struct{}{}
	project, err := opts.ToProject(ctx, dockerCli, services, cli.WithoutEnvironmentResolution)
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
		fmt.Fprintln(dockerCli.Out(), p)
	}
	return nil
}

func runConfigImages(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	project, err := opts.ToProject(ctx, dockerCli, services, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}

	for _, s := range project.Services {
		fmt.Fprintln(dockerCli.Out(), api.GetImageNameOrDefault(s, project.Name))
	}
	return nil
}

func runVariables(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	opts.noInterpolate = true
	model, err := opts.ToModel(ctx, dockerCli, services, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}

	variables := template.ExtractVariables(model, template.DefaultPattern)

	return formatter.Print(variables, "", dockerCli.Out(), func(w io.Writer) {
		for name, variable := range variables {
			_, _ = fmt.Fprintf(w, "%s\t%t\t%s\t%s\n", name, variable.Required, variable.DefaultValue, variable.PresenceValue)
		}
	}, "NAME", "REQUIRED", "DEFAULT VALUE", "ALTERNATE VALUE")
}

func runEnvironment(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	project, err := opts.ToProject(ctx, dockerCli, services)
	if err != nil {
		return err
	}

	for _, v := range project.Environment.Values() {
		fmt.Println(v)
	}
	return nil
}

func escapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}
