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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/platforms"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/docker/cli/cli/command"
	cliformatter "github.com/docker/cli/cli/command/formatter"
	cliflags "github.com/docker/cli/cli/flags"
)

const (
	defaultImageTableFormat = "table {{.ContainerName}}\t{{.Repository}}\t{{.Tag}}\t{{.Platform}}\t{{.ID}}\t{{.Size}}\t{{.Created}}"
)

type imageOptions struct {
	*ProjectOptions
	Quiet   bool
	Format  string
	noTrunc bool
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
	imgCmd.Flags().StringVar(&opts.Format, "format", "table", cliflags.FormatHelp)
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

	var format cliformatter.Format

	if opts.Format == cliformatter.TableFormatKey {
		format = cliformatter.Format(defaultImageTableFormat)
	} else {
		format = cliformatter.Format(opts.Format)
	}

	imageCtx := cliformatter.Context{
		Output: dockerCli.Out(),
		Format: format,
		Trunc:  false,
	}

	return ImageWrite(imageCtx, images)
}

// ImageWrite renders the context for a list of images
func ImageWrite(ctx cliformatter.Context, images map[string]api.ImageSummary) error {
	render := func(format func(subContext cliformatter.SubContext) error) error {
		for container, image := range images {
			err := format(&ImageContext{i: image, container: container, cliFormat: ctx.Format})
			if err != nil {
				return err
			}
		}
		return nil
	}
	return ctx.Write(NewImageContext(), render)
}

// ImageContext is a struct used for rendering a list of images in a Go template.
type ImageContext struct {
	cliformatter.HeaderContext
	i         api.ImageSummary
	container string
	cliFormat cliformatter.Format
}

func NewImageContext() *ImageContext {
	imageCtx := ImageContext{}
	imageCtx.Header = cliformatter.SubHeaderContext{
		"ID":            "IMAGE ID",
		"ContainerName": "CONTAINER",
		"Repository":    "REPOSITORY",
		"Tag":           "TAG",
		"Size":          "SIZE",
		"Platform":      "PLATFORM",
		"LastTagTime":   "LAST TAG TIME",
		"Created":       "CREATED",
	}
	return &imageCtx
}

func (i *ImageContext) MarshalJSON() ([]byte, error) {
	return cliformatter.MarshalJSON(i)
}

func (i *ImageContext) ID() string {
	if i.cliFormat.IsJSON() {
		return i.i.ID
	}
	return stringid.TruncateID(i.i.ID)
}

func (i *ImageContext) ContainerName() string {
	return i.container
}

func (i *ImageContext) Repository() string {
	repo := i.i.Repository
	if repo == "" {
		repo = "<none>"
	}
	return repo
}

func (i *ImageContext) Tag() string {
	tag := i.i.Tag
	if tag == "" {
		tag = "<none>"
	}
	return tag
}

func (i *ImageContext) Size() string {
	if i.cliFormat.IsJSON() {
		return strconv.FormatInt(i.i.Size, 10)
	}
	return units.HumanSizeWithPrecision(float64(i.i.Size), 3)
}

func (i *ImageContext) LastTagTime() string {
	return i.i.LastTagTime.String()
}

func (i *ImageContext) Created() string {
	return units.HumanDuration(time.Now().UTC().Sub(i.i.LastTagTime)) + " ago"
}

func (i *ImageContext) Platform() string {
	return platforms.Format(i.i.Platform)
}
