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
	"errors"

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type publishOptions struct {
	*ProjectOptions
	resolveImageDigests bool
	ociVersion          string
	withEnvironment     bool
	assumeYes           bool
	app                 bool
	insecureRegistry    bool
}

func publishCommand(p *ProjectOptions, dockerCli command.Cli, backendOptions *BackendOptions) *cobra.Command {
	opts := publishOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "publish [OPTIONS] REPOSITORY[:TAG]",
		Short: "Publish compose application",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPublish(ctx, dockerCli, backendOptions, opts, args[0])
		}),
		Args: cli.ExactArgs(1),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests")
	flags.StringVar(&opts.ociVersion, "oci-version", "", "OCI image/artifact specification version (automatically determined by default)")
	flags.BoolVar(&opts.withEnvironment, "with-env", false, "Include environment variables in the published OCI artifact")
	flags.BoolVarP(&opts.assumeYes, "yes", "y", false, `Assume "yes" as answer to all prompts`)
	flags.BoolVar(&opts.app, "app", false, "Published compose application (includes referenced images)")
	flags.BoolVar(&opts.insecureRegistry, "insecure-registry", false, "Use insecure registry")
	flags.SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		// assumeYes was introduced by mistake as `--y`
		if name == "y" {
			logrus.Warn("--y is deprecated, please use --yes instead")
			name = "yes"
		}
		return pflag.NormalizedName(name)
	})
	// Should **only** be used for testing purpose, we don't want to promote use of insecure registries
	_ = flags.MarkHidden("insecure-registry")

	return cmd
}

func runPublish(ctx context.Context, dockerCli command.Cli, backendOptions *BackendOptions, opts publishOptions, repository string) error {
	if opts.assumeYes {
		backendOptions.Options = append(backendOptions.Options, compose.WithPrompt(compose.AlwaysOkPrompt()))
	}

	backend, err := compose.NewComposeService(dockerCli, backendOptions.Options...)
	if err != nil {
		return err
	}

	project, metrics, err := opts.ToProject(ctx, dockerCli, backend, nil)
	if err != nil {
		return err
	}

	if metrics.CountIncludesLocal > 0 {
		return errors.New("cannot publish compose file with local includes")
	}

	return backend.Publish(ctx, project, repository, api.PublishOptions{
		ResolveImageDigests: opts.resolveImageDigests || opts.app,
		Application:         opts.app,
		OCIVersion:          api.OCIVersion(opts.ociVersion),
		WithEnvironment:     opts.withEnvironment,
		InsecureRegistry:    opts.insecureRegistry,
	})
}
