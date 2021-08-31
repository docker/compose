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
	"os"
	"sort"
	"strings"

	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

type imageOptions struct {
	*projectOptions
	Quiet bool
}

func imagesCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := imageOptions{
		projectOptions: p,
	}
	imgCmd := &cobra.Command{
		Use:   "images [SERVICE...]",
		Short: "List images used by the created containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runImages(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	imgCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	return imgCmd
}

func runImages(ctx context.Context, backend api.Service, opts imageOptions, services []string) error {
	projectName, err := opts.toProjectName()
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
			if !utils.StringContains(ids, id) {
				ids = append(ids, id)
			}
		}
		for _, img := range ids {
			fmt.Println(img)
		}
		return nil
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].ContainerName < images[j].ContainerName
	})

	return formatter.Print(images, formatter.PRETTY, os.Stdout,
		func(w io.Writer) {
			for _, img := range images {
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
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", img.ContainerName, repo, tag, id, size)
			}
		},
		"Container", "Repository", "Tag", "Image Id", "Size")
}
