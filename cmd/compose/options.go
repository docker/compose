/*
   Copyright 2023 Docker Compose CLI authors

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
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/template"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/internal/tracing"
	ui "github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/prompt"
)

func applyPlatforms(project *types.Project, buildForSinglePlatform bool) error {
	defaultPlatform := project.Environment["DOCKER_DEFAULT_PLATFORM"]
	for name, service := range project.Services {
		if service.Build == nil {
			continue
		}

		// default platform only applies if the service doesn't specify
		if defaultPlatform != "" && service.Platform == "" {
			if len(service.Build.Platforms) > 0 && !slices.Contains(service.Build.Platforms, defaultPlatform) {
				return fmt.Errorf("service %q build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: %s", name, defaultPlatform)
			}
			service.Platform = defaultPlatform
		}

		if service.Platform != "" {
			if len(service.Build.Platforms) > 0 {
				if !slices.Contains(service.Build.Platforms, service.Platform) {
					return fmt.Errorf("service %q build configuration does not support platform: %s", name, service.Platform)
				}
			}

			if buildForSinglePlatform || len(service.Build.Platforms) == 0 {
				// if we're building for a single platform, we want to build for the platform we'll use to run the image
				// similarly, if no build platforms were explicitly specified, it makes sense to build for the platform
				// the image is designed for rather than allowing the builder to infer the platform
				service.Build.Platforms = []string{service.Platform}
			}
		}

		// services can specify that they should be built for multiple platforms, which can be used
		// with `docker compose build` to produce a multi-arch image
		// other cases, such as `up` and `run`, need a single architecture to actually run
		// if there is only a single platform present (which might have been inferred
		// from service.Platform above), it will be used, even if it requires emulation.
		// if there's more than one platform, then the list is cleared so that the builder
		// can decide.
		// TODO(milas): there's no validation that the platform the builder will pick is actually one
		// 	of the supported platforms from the build definition
		// 	e.g. `build.platforms: [linux/arm64, linux/amd64]` on a `linux/ppc64` machine would build
		// 	for `linux/ppc64` instead of returning an error that it's not a valid platform for the service.
		if buildForSinglePlatform && len(service.Build.Platforms) > 1 {
			// empty indicates that the builder gets to decide
			service.Build.Platforms = nil
		}
		project.Services[name] = service
	}
	return nil
}

// isRemoteConfig checks if the main compose file is from a remote source (OCI or Git)
func isRemoteConfig(dockerCli command.Cli, options buildOptions) bool {
	if len(options.ConfigPaths) == 0 {
		return false
	}
	remoteLoaders := options.remoteLoaders(dockerCli)
	for _, loader := range remoteLoaders {
		if loader.Accept(options.ConfigPaths[0]) {
			return true
		}
	}
	return false
}

// checksForRemoteStack handles environment variable prompts for remote configurations
func checksForRemoteStack(ctx context.Context, dockerCli command.Cli, project *types.Project, options buildOptions, assumeYes bool, cmdEnvs []string) error {
	if !isRemoteConfig(dockerCli, options) {
		return nil
	}
	if metrics, ok := ctx.Value(tracing.MetricsKey{}).(tracing.Metrics); ok && metrics.CountIncludesRemote > 0 {
		if err := confirmRemoteIncludes(dockerCli, options, assumeYes); err != nil {
			return err
		}
	}
	displayLocationRemoteStack(dockerCli, project, options)
	return promptForInterpolatedVariables(ctx, dockerCli, options.ProjectOptions, assumeYes, cmdEnvs)
}

// Prepare the values map and collect all variables info
type varInfo struct {
	name         string
	value        string
	source       string
	required     bool
	defaultValue string
}

// promptForInterpolatedVariables displays all variables and their values at once,
// then prompts for confirmation
func promptForInterpolatedVariables(ctx context.Context, dockerCli command.Cli, projectOptions *ProjectOptions, assumeYes bool, cmdEnvs []string) error {
	if assumeYes {
		return nil
	}

	varsInfo, noVariables, err := extractInterpolationVariablesFromModel(ctx, dockerCli, projectOptions, cmdEnvs)
	if err != nil {
		return err
	}

	if noVariables {
		return nil
	}

	displayInterpolationVariables(dockerCli.Out(), varsInfo)

	// Prompt for confirmation
	userInput := prompt.NewPrompt(dockerCli.In(), dockerCli.Out())
	msg := "\nDo you want to proceed with these variables? [Y/n]: "
	confirmed, err := userInput.Confirm(msg, true)
	if err != nil {
		return err
	}

	if !confirmed {
		return fmt.Errorf("operation cancelled by user")
	}

	return nil
}

func extractInterpolationVariablesFromModel(ctx context.Context, dockerCli command.Cli, projectOptions *ProjectOptions, cmdEnvs []string) ([]varInfo, bool, error) {
	cmdEnvMap := extractEnvCLIDefined(cmdEnvs)

	// Create a model without interpolation to extract variables
	opts := configOptions{
		noInterpolate:  true,
		ProjectOptions: projectOptions,
	}

	model, err := opts.ToModel(ctx, dockerCli, nil, cli.WithoutEnvironmentResolution)
	if err != nil {
		return nil, false, err
	}

	// Extract variables that need interpolation
	variables := template.ExtractVariables(model, template.DefaultPattern)
	if len(variables) == 0 {
		return nil, true, nil
	}

	var varsInfo []varInfo
	proposedValues := make(map[string]string)

	for name, variable := range variables {
		info := varInfo{
			name:         name,
			required:     variable.Required,
			defaultValue: variable.DefaultValue,
		}

		// Determine value and source based on priority
		if value, exists := cmdEnvMap[name]; exists {
			info.value = value
			info.source = "command-line"
			proposedValues[name] = value
		} else if value, exists := os.LookupEnv(name); exists {
			info.value = value
			info.source = "environment"
			proposedValues[name] = value
		} else if variable.DefaultValue != "" {
			info.value = variable.DefaultValue
			info.source = "compose file"
			proposedValues[name] = variable.DefaultValue
		} else {
			info.value = "<unset>"
			info.source = "none"
		}

		varsInfo = append(varsInfo, info)
	}
	return varsInfo, false, nil
}

func extractEnvCLIDefined(cmdEnvs []string) map[string]string {
	// Parse command-line environment variables
	cmdEnvMap := make(map[string]string)
	for _, env := range cmdEnvs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			cmdEnvMap[parts[0]] = parts[1]
		}
	}
	return cmdEnvMap
}

func displayInterpolationVariables(writer io.Writer, varsInfo []varInfo) {
	// Display all variables in a table format
	_, _ = fmt.Fprintln(writer, "\nFound the following variables in configuration:")

	w := tabwriter.NewWriter(writer, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "VARIABLE\tVALUE\tSOURCE\tREQUIRED\tDEFAULT")
	sort.Slice(varsInfo, func(a, b int) bool {
		return varsInfo[a].name < varsInfo[b].name
	})
	for _, info := range varsInfo {
		required := "no"
		if info.required {
			required = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			info.name,
			info.value,
			info.source,
			required,
			info.defaultValue,
		)
	}
	_ = w.Flush()
}

func displayLocationRemoteStack(dockerCli command.Cli, project *types.Project, options buildOptions) {
	mainComposeFile := options.ProjectOptions.ConfigPaths[0] //nolint:staticcheck
	if ui.Mode != ui.ModeQuiet && ui.Mode != ui.ModeJSON {
		_, _ = fmt.Fprintf(dockerCli.Out(), "Your compose stack %q is stored in %q\n", mainComposeFile, project.WorkingDir)
	}
}

func confirmRemoteIncludes(dockerCli command.Cli, options buildOptions, assumeYes bool) error {
	if assumeYes {
		return nil
	}

	var remoteIncludes []string
	remoteLoaders := options.ProjectOptions.remoteLoaders(dockerCli) //nolint:staticcheck
	for _, cf := range options.ProjectOptions.ConfigPaths {          //nolint:staticcheck
		for _, loader := range remoteLoaders {
			if loader.Accept(cf) {
				remoteIncludes = append(remoteIncludes, cf)
				break
			}
		}
	}

	if len(remoteIncludes) == 0 {
		return nil
	}

	_, _ = fmt.Fprintln(dockerCli.Out(), "\nWarning: This Compose project includes files from remote sources:")
	for _, include := range remoteIncludes {
		_, _ = fmt.Fprintf(dockerCli.Out(), "  - %s\n", include)
	}
	_, _ = fmt.Fprintln(dockerCli.Out(), "\nRemote includes could potentially be malicious. Make sure you trust the source.")

	msg := "Do you want to continue? [y/N]: "
	confirmed, err := prompt.NewPrompt(dockerCli.In(), dockerCli.Out()).Confirm(msg, false)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("operation cancelled by user")
	}

	return nil
}
