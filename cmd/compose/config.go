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
	"slices"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/template"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
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
	noResolveEnv        bool
	services            bool
	volumes             bool
	networks            bool
	models              bool
	profiles            bool
	images              bool
	hash                string
	noConsistency       bool
	variables           bool
	environment         bool
	lockImageDigests    bool
}

func (o *configOptions) ToProject(ctx context.Context, dockerCli command.Cli, backend api.Compose, services []string) (*types.Project, error) {
	project, _, err := o.ProjectOptions.ToProject(ctx, dockerCli, backend, services, o.toProjectOptionsFns()...)
	return project, err
}

func (o *configOptions) ToModel(ctx context.Context, dockerCli command.Cli, services []string, po ...cli.ProjectOptionsFn) (map[string]any, error) {
	po = append(po, o.toProjectOptionsFns()...)
	return o.ProjectOptions.ToModel(ctx, dockerCli, services, po...)
}

// toProjectOptionsFns converts config options to cli.ProjectOptionsFn
func (o *configOptions) toProjectOptionsFns() []cli.ProjectOptionsFn {
	fns := []cli.ProjectOptionsFn{
		cli.WithInterpolation(!o.noInterpolate),
		cli.WithResolvedPaths(!o.noResolvePath),
		cli.WithNormalization(!o.noNormalize),
		cli.WithConsistency(!o.noConsistency),
		cli.WithDefaultProfiles(o.Profiles...),
		cli.WithDiscardEnvFile,
	}
	if o.noResolveEnv {
		fns = append(fns, cli.WithoutEnvironmentResolution)
	}
	return fns
}

func configCommand(p *ProjectOptions, dockerCli command.Cli) *cobra.Command {
	opts := configOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "config [OPTIONS] [SERVICE...]",
		Short: "Parse, resolve and render compose file in canonical format",
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
			if opts.lockImageDigests {
				opts.resolveImageDigests = true
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
			if opts.networks {
				return runNetworks(ctx, dockerCli, opts)
			}
			if opts.models {
				return runModels(ctx, dockerCli, opts)
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

			if opts.Format == "" {
				opts.Format = "yaml"
			}
			return runConfig(ctx, dockerCli, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "", "Format the output. Values: [yaml | json]")
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests")
	flags.BoolVar(&opts.lockImageDigests, "lock-image-digests", false, "Produces an override file with image digests")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only validate the configuration, don't print anything")
	flags.BoolVar(&opts.noInterpolate, "no-interpolate", false, "Don't interpolate environment variables")
	flags.BoolVar(&opts.noNormalize, "no-normalize", false, "Don't normalize compose model")
	flags.BoolVar(&opts.noResolvePath, "no-path-resolution", false, "Don't resolve file paths")
	flags.BoolVar(&opts.noConsistency, "no-consistency", false, "Don't check model consistency - warning: may produce invalid Compose output")
	flags.BoolVar(&opts.noResolveEnv, "no-env-resolution", false, "Don't resolve service env files")

	flags.BoolVar(&opts.services, "services", false, "Print the service names, one per line.")
	flags.BoolVar(&opts.volumes, "volumes", false, "Print the volume names, one per line.")
	flags.BoolVar(&opts.networks, "networks", false, "Print the network names, one per line.")
	flags.BoolVar(&opts.models, "models", false, "Print the model names, one per line.")
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
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return nil, err
	}

	project, err := opts.ToProject(ctx, dockerCli, backend, services)
	if err != nil {
		return nil, err
	}

	if opts.resolveImageDigests {
		project, err = project.WithImagesResolved(compose.ImageDigestResolver(ctx, dockerCli.ConfigFile(), dockerCli.Client()))
		if err != nil {
			return nil, err
		}
	}

	if !opts.noResolveEnv {
		project, err = project.WithServicesEnvironmentResolved(true)
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

	if opts.lockImageDigests {
		warnHooksNotLockable(project)
		project = imagesOnly(project)
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

// imagesOnly return project with all attributes removed but service.images and `type: image` volumes
func imagesOnly(project *types.Project) *types.Project {
	digests := types.Services{}
	for name, config := range project.Services {
		service := types.ServiceConfig{
			Image: config.Image,
		}
		for _, vol := range config.Volumes {
			if vol.Type == types.VolumeTypeImage {
				service.Volumes = append(service.Volumes, vol)
			}
		}
		digests[name] = service
	}
	project = &types.Project{Services: digests}
	return project
}

func runConfigNoInterpolate(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) ([]byte, error) {
	if len(services) > 0 {
		logrus.Warn("service filtering is not applied when --no-interpolate is set, the full model will be rendered")
	}
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

	if opts.lockImageDigests {
		warnModelHooksNotLockable(model)
		lockModel(model)
	}

	return formatModel(model, opts.Format)
}

// hook sequences are appended when compose files are merged, so a lock override
// cannot pin a hook image without duplicating the hook: the lock file leaves
// pre_start hooks out, and their images stay unpinned once merged
const hooksNotLockableWarning = "service %q: pre_start hook images are not pinned in the override file produced by --lock-image-digests, use the full --resolve-image-digests output to pin them"

func warnHooksNotLockable(project *types.Project) {
	for name, service := range project.Services {
		for _, hook := range service.PreStart {
			if hook.Image != "" {
				logrus.Warnf(hooksNotLockableWarning, name)
				break
			}
		}
	}
}

func warnModelHooksNotLockable(model map[string]any) {
	services, ok := model["services"].(map[string]any)
	if !ok {
		return
	}
	for name, s := range services {
		service, ok := s.(map[string]any)
		if !ok {
			continue
		}
		for _, hook := range preStartHooks(service) {
			if image, ok := hook["image"].(string); ok && image != "" {
				logrus.Warnf(hooksNotLockableWarning, name)
				break
			}
		}
	}
}

// lockModel removes from model all attributes but service images and `type: image` volumes
func lockModel(model map[string]any) {
	for key, e := range model {
		if key != "services" {
			delete(model, key)
			continue
		}
		for _, s := range e.(map[string]any) {
			service := s.(map[string]any)
			for key := range service {
				switch key {
				case "image":
				case "volumes":
					if volumes := imageVolumes(service); len(volumes) > 0 {
						// write back as []any to keep the raw-model volumes type unchanged
						filtered := make([]any, len(volumes))
						for i, volume := range volumes {
							filtered[i] = volume
						}
						service["volumes"] = filtered
					} else {
						delete(service, "volumes")
					}
				default:
					delete(service, key)
				}
			}
		}
	}
}

func resolveImageDigests(ctx context.Context, dockerCli command.Cli, model map[string]any) error {
	// create a pseudo-project so we can rely on WithImagesResolved to resolve images,
	// pre_start hook images and `type: image` volume sources, keyed by actual service
	// names so sources referencing another service are detected as such and kept unresolved
	p := &types.Project{
		Services: types.Services{},
	}
	services, ok := model["services"].(map[string]any)
	if !ok {
		// services is optional at the top level of the compose model
		return nil
	}
	for name, s := range services {
		service := s.(map[string]any)
		config := types.ServiceConfig{}
		if image, ok := service["image"].(string); ok {
			config.Image = image
		}
		for _, hook := range preStartHooks(service) {
			image, _ := hook["image"].(string)
			config.PreStart = append(config.PreStart, types.ServiceHook{Image: image})
		}
		for _, volume := range imageVolumes(service) {
			source, _ := volume["source"].(string)
			config.Volumes = append(config.Volumes, types.ServiceVolumeConfig{
				Type:   types.VolumeTypeImage,
				Source: source,
			})
		}
		p.Services[name] = config
	}

	p, err := p.WithImagesResolved(compose.ImageDigestResolver(ctx, dockerCli.ConfigFile(), dockerCli.Client()))
	if err != nil {
		return err
	}

	// update model with image and volume-source references resolved with digest;
	// fields absent from the resolved pseudo-project (empty Image / Source) are left untouched
	for name, s := range services {
		service := s.(map[string]any)
		config := p.Services[name]
		if config.Image != "" {
			service["image"] = config.Image
		}
		for i, hook := range preStartHooks(service) {
			if image := config.PreStart[i].Image; image != "" {
				hook["image"] = image
			}
		}
		for i, volume := range imageVolumes(service) {
			if source := config.Volumes[i].Source; source != "" {
				volume["source"] = source
			}
		}
	}
	return nil
}

// preStartHooks returns the pre_start hook declarations of a service raw model
func preStartHooks(service map[string]any) []map[string]any {
	hooks, ok := service["pre_start"].([]any)
	if !ok {
		return nil
	}
	var result []map[string]any
	for _, h := range hooks {
		if hook, ok := h.(map[string]any); ok {
			result = append(result, hook)
		}
	}
	return result
}

// imageVolumes returns the `type: image` volume declarations of a service raw model
func imageVolumes(service map[string]any) []map[string]any {
	volumes, ok := service["volumes"].([]any)
	if !ok {
		return nil
	}
	var images []map[string]any
	for _, v := range volumes {
		volume, ok := v.(map[string]any)
		if ok && volume["type"] == types.VolumeTypeImage {
			images = append(images, volume)
		}
	}
	return images
}

func formatModel(model map[string]any, format string) (content []byte, err error) {
	switch format {
	case "json":
		return json.MarshalIndent(model, "", "  ")
	case "yaml":
		buf := bytes.NewBuffer([]byte{})
		encoder := yaml.NewEncoder(buf)
		encoder.SetIndent(2)
		err = encoder.Encode(model)
		return buf.Bytes(), err
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func runServices(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	if opts.noInterpolate {
		// we can't use ToProject, so the model we render here is only partially resolved
		data, err := opts.ToModel(ctx, dockerCli, nil, cli.WithoutEnvironmentResolution)
		if err != nil {
			return err
		}

		if _, ok := data["services"]; ok {
			for serviceName := range data["services"].(map[string]any) {
				_, _ = fmt.Fprintln(dockerCli.Out(), serviceName)
			}
		}

		return nil
	}

	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, _, err := opts.ProjectOptions.ToProject(ctx, dockerCli, backend, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	err = project.ForEachService(project.ServiceNames(), func(serviceName string, _ *types.ServiceConfig) error {
		_, _ = fmt.Fprintln(dockerCli.Out(), serviceName)
		return nil
	})

	return err
}

func runVolumes(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, _, err := opts.ProjectOptions.ToProject(ctx, dockerCli, backend, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	for n := range project.Volumes {
		_, _ = fmt.Fprintln(dockerCli.Out(), n)
	}
	return nil
}

func runNetworks(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, _, err := opts.ProjectOptions.ToProject(ctx, dockerCli, backend, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	for n := range project.Networks {
		_, _ = fmt.Fprintln(dockerCli.Out(), n)
	}
	return nil
}

func runModels(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, _, err := opts.ProjectOptions.ToProject(ctx, dockerCli, backend, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}
	for _, model := range project.Models {
		if model.Model != "" {
			_, _ = fmt.Fprintln(dockerCli.Out(), model.Model)
		}
	}
	return nil
}

func runHash(ctx context.Context, dockerCli command.Cli, opts configOptions) error {
	var services []string
	if opts.hash != "*" {
		services = append(services, strings.Split(opts.hash, ",")...)
	}

	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, _, err := opts.ProjectOptions.ToProject(ctx, dockerCli, backend, nil, cli.WithoutEnvironmentResolution)
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
	slices.Sort(sorted)

	for _, name := range sorted {
		s, err := project.GetService(name)
		if err != nil {
			return err
		}

		hash, err := compose.ServiceHash(s)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(dockerCli.Out(), "%s %s\n", name, hash)
	}
	return nil
}

func runProfiles(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	set := map[string]struct{}{}

	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, err := opts.ToProject(ctx, dockerCli, backend, services)
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
		_, _ = fmt.Fprintln(dockerCli.Out(), p)
	}
	return nil
}

func runConfigImages(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, err := opts.ToProject(ctx, dockerCli, backend, services)
	if err != nil {
		return err
	}

	for _, s := range project.Services {
		_, _ = fmt.Fprintln(dockerCli.Out(), api.GetImageNameOrDefault(s, project.Name))
	}
	return nil
}

func runVariables(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	if len(services) > 0 {
		logrus.Warn("service filtering is not applied when --variables is set, variables from the full model will be rendered")
	}
	opts.noInterpolate = true
	model, err := opts.ToModel(ctx, dockerCli, services, cli.WithoutEnvironmentResolution, cli.WithLoadOptions(loader.WithSkipValidation))
	if err != nil {
		return err
	}

	variables := template.ExtractVariables(model, template.DefaultPattern)

	if opts.Format == "yaml" {
		result, err := yaml.Marshal(variables)
		if err != nil {
			return err
		}
		fmt.Print(string(result))
		return nil
	}

	return formatter.Print(variables, opts.Format, dockerCli.Out(), func(w io.Writer) {
		for name, variable := range variables {
			_, _ = fmt.Fprintf(w, "%s\t%t\t%s\t%s\n", name, variable.Required, variable.DefaultValue, variable.PresenceValue)
		}
	}, "NAME", "REQUIRED", "DEFAULT VALUE", "ALTERNATE VALUE")
}

func runEnvironment(ctx context.Context, dockerCli command.Cli, opts configOptions, services []string) error {
	backend, err := compose.NewComposeService(dockerCli)
	if err != nil {
		return err
	}

	project, err := opts.ToProject(ctx, dockerCli, backend, services)
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
