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
	"fmt"
	"io"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/command"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/bridge"
)

func bridgeCommand(p *ProjectOptions, dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "bridge CMD [OPTIONS]",
		Short:            "Convert compose files into another model",
		TraverseChildren: true,
	}
	cmd.AddCommand(
		convertCommand(p, dockerCli),
		transformersCommand(dockerCli),
	)
	return cmd
}

func convertCommand(p *ProjectOptions, dockerCli command.Cli) *cobra.Command {
	convertOpts := bridge.ConvertOptions{}
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert compose files to Kubernetes manifests, Helm charts, or another model",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runConvert(ctx, dockerCli, p, convertOpts)
		}),
	}
	flags := cmd.Flags()
	flags.StringVarP(&convertOpts.Output, "output", "o", "out", "The output directory for the Kubernetes resources")
	flags.StringArrayVarP(&convertOpts.Transformations, "transformation", "t", nil, "Transformation to apply to compose model (default: docker/compose-bridge-kubernetes)")
	flags.StringVar(&convertOpts.Templates, "templates", "", "Directory containing transformation templates")
	return cmd
}

func runConvert(ctx context.Context, dockerCli command.Cli, p *ProjectOptions, opts bridge.ConvertOptions) error {
	project, _, err := p.ToProject(ctx, dockerCli, nil)
	if err != nil {
		return err
	}
	return bridge.Convert(ctx, dockerCli, project, opts)
}

func transformersCommand(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transformations CMD [OPTIONS]",
		Short: "Manage transformation images",
	}
	cmd.AddCommand(
		listTransformersCommand(dockerCli),
		createTransformerCommand(dockerCli),
	)
	return cmd
}

func listTransformersCommand(dockerCli command.Cli) *cobra.Command {
	options := lsOptions{}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List available transformations",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			transformers, err := bridge.ListTransformers(ctx, dockerCli)
			if err != nil {
				return err
			}
			return displayTransformer(dockerCli, transformers, options)
		}),
	}
	cmd.Flags().StringVar(&options.Format, "format", "table", "Format the output. Values: [table | json]")
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Only display transformer names")
	return cmd
}

func displayTransformer(dockerCli command.Cli, transformers []image.Summary, options lsOptions) error {
	if options.Quiet {
		for _, t := range transformers {
			if len(t.RepoTags) > 0 {
				_, _ = fmt.Fprintln(dockerCli.Out(), t.RepoTags[0])
			} else {
				_, _ = fmt.Fprintln(dockerCli.Out(), t.ID)
			}
		}
		return nil
	}
	return formatter.Print(transformers, options.Format, dockerCli.Out(),
		func(w io.Writer) {
			for _, img := range transformers {
				id := stringid.TruncateID(img.ID)
				size := units.HumanSizeWithPrecision(float64(img.Size), 3)
				repo, tag := "<none>", "<none>"
				if len(img.RepoTags) > 0 {
					ref, err := reference.ParseDockerRef(img.RepoTags[0])
					if err == nil {
						// ParseDockerRef will reject a local image ID
						repo = reference.FamiliarName(ref)
						if tagged, ok := ref.(reference.Tagged); ok {
							tag = tagged.Tag()
						}
					}
				}

				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id, repo, tag, size)
			}
		},
		"IMAGE ID", "REPO", "TAGS", "SIZE")
}

func createTransformerCommand(dockerCli command.Cli) *cobra.Command {
	var opts bridge.CreateTransformerOptions
	cmd := &cobra.Command{
		Use:   "create [OPTION] PATH",
		Short: "Create a new transformation",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			opts.Dest = args[0]
			return bridge.CreateTransformer(ctx, dockerCli, opts)
		}),
	}
	cmd.Flags().StringVarP(&opts.From, "from", "f", "", "Existing transformation to copy (default: docker/compose-bridge-kubernetes)")
	return cmd
}
