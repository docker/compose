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

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type publishOptions struct {
	*ProjectOptions
	resolveImageDigests bool
	ociVersion          string
}

func publishCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := publishOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "publish [OPTIONS] REPOSITORY[:TAG]",
		Short: "Publish compose application",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPublish(ctx, dockerCli, backend, opts, args[0])
		}),
		Args: cobra.ExactArgs(1),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.resolveImageDigests, "resolve-image-digests", false, "Pin image tags to digests")
	flags.StringVar(&opts.ociVersion, "oci-version", "", "OCI Image/Artifact specification version (automatically determined by default)")
	return cmd
}

func runPublish(ctx context.Context, dockerCli command.Cli, backend api.Service, opts publishOptions, repository string) error {
	project, _, err := opts.ToProject(ctx, dockerCli, nil)
	if err != nil {
		return err
	}

	return backend.Publish(ctx, project, repository, api.PublishOptions{
		ResolveImageDigests: opts.resolveImageDigests,
		OCIVersion:          api.OCIVersion(opts.ociVersion),
	})
}
