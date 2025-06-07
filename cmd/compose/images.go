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
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/containerd/platforms"
	"github.com/docker/cli/cli/command"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
)

type imageOptions struct {
	*ProjectOptions
	Quiet  bool
	Format string
}

func imagesCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := imageOptions{
		ProjectOptions: p,
	}
	imgCmd := &cobra.Command{
		Use:   "images [OPTIONS] [SERVICE...]",
		Short: "List images used by the created containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runImages(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	imgCmd.Flags().StringVar(&opts.Format, "format", "table", "Format the output. Values: [table | json]")
	imgCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	return imgCmd
}

func runImages(ctx context.Context, dockerCli command.Cli, backend api.Service, opts imageOptions, services []string) error {
	projectName, err := opts.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}

	images, err := backend.Images(ctx, projectName, api.ImagesOptions{
		Services: services,
	})
	if err != nil {
		return err
	}

	if opts.Quiet {
		ids := []string{}
		for _, img := range images {
			id := img.ID
			if i := strings.IndexRune(img.ID, ':'); i >= 0 {
				id = id[i+1:]
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		for _, img := range ids {
			_, _ = fmt.Fprintln(dockerCli.Out(), img)
		}
		return nil
	}
	if opts.Format == "json" {
		// Convert map to slice
		var imageList []api.ImageSummary
		for _, img := range images {
			imageList = append(imageList, img)
		}
		return formatter.Print(imageList, opts.Format, dockerCli.Out(), nil)
	}

	return formatter.Print(images, opts.Format, dockerCli.Out(),
		func(w io.Writer) {
			for _, container := range slices.Sorted(maps.Keys(images)) {
				img := images[container]
				id := stringid.TruncateID(img.ID)
				size := units.HumanSizeWithPrecision(float64(img.Size), 3)
				repo := img.Repository
				if repo == "" {
					repo = "<none>"
				}
				tag := img.Tag
				if tag == "" {
					tag = "<none>"
				}
				created := units.HumanDuration(time.Now().UTC().Sub(img.LastTagTime)) + " ago"
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					container, repo, tag, platforms.Format(img.Platform), id, size, created)
			}
		},
		"CONTAINER", "REPOSITORY", "TAG", "PLATFORM", "IMAGE ID", "SIZE", "CREATED")
}
