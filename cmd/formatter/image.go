/*
   Copyright 2026 Docker Compose CLI authors

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

package formatter

import (
	"time"

	"github.com/containerd/platforms"
	cliformatter "github.com/docker/cli/cli/command/formatter"
	"github.com/docker/go-units"
	"github.com/moby/moby/client/pkg/stringid"

	"github.com/docker/compose/v5/pkg/api"
)

const (
	defaultComposeImageTableFormat = "table {{.ContainerName}}\t{{.Repository}}\t{{.Tag}}\t{{.Platform}}\t{{.ID}}\t{{.Size}}\t{{.Created}}"

	composeImageContainerHeader = "CONTAINER"
	composeImageIDHeader        = "IMAGE ID"
	composeImageRepository      = "REPOSITORY"
	composeImageTag             = "TAG"
	composeImagePlatform        = "PLATFORM"
	composeImageSize            = "SIZE"
	composeImageCreated         = "CREATED"
	composeImageCreatedAt       = "CREATED AT"
	composeImageLastTagTime     = "LAST TAG TIME"
)

// Image is the display model for docker compose images.
type Image struct {
	ContainerName string
	Summary       api.ImageSummary
}

// NewImageFormat returns a Docker CLI formatter format for Compose images.
func NewImageFormat(source string) cliformatter.Format {
	switch source {
	case cliformatter.TableFormatKey, "":
		return cliformatter.Format(defaultComposeImageTableFormat)
	case cliformatter.RawFormatKey:
		return `container_name: {{.ContainerName}}
repository: {{.Repository}}
tag: {{.Tag}}
platform: {{.Platform}}
image_id: {{.ID}}
size: {{.Size}}
created: {{.Created}}
`
	default:
		return cliformatter.Format(source)
	}
}

// ImageWrite writes formatted Compose images using Docker CLI templates.
func ImageWrite(ctx cliformatter.Context, images []Image) error {
	render := func(format func(cliformatter.SubContext) error) error {
		for _, image := range images {
			if err := format(&imageContext{image: image}); err != nil {
				return err
			}
		}
		return nil
	}
	return ctx.Write(newImageContext(), render)
}

type imageContext struct {
	cliformatter.HeaderContext
	image Image
}

func newImageContext() *imageContext {
	imageCtx := imageContext{}
	imageCtx.Header = cliformatter.SubHeaderContext{
		"ContainerName": composeImageContainerHeader,
		"ID":            composeImageIDHeader,
		"Repository":    composeImageRepository,
		"Tag":           composeImageTag,
		"Platform":      composeImagePlatform,
		"Size":          composeImageSize,
		"Created":       composeImageCreated,
		"CreatedAt":     composeImageCreatedAt,
		"LastTagTime":   composeImageLastTagTime,
	}
	return &imageCtx
}

func (c *imageContext) MarshalJSON() ([]byte, error) {
	return cliformatter.MarshalJSON(c)
}

func (c *imageContext) ContainerName() string {
	return c.image.ContainerName
}

func (c *imageContext) ID() string {
	return stringid.TruncateID(c.image.Summary.ID)
}

func (c *imageContext) Repository() string {
	if c.image.Summary.Repository == "" {
		return "<none>"
	}
	return c.image.Summary.Repository
}

func (c *imageContext) Tag() string {
	if c.image.Summary.Tag == "" {
		return "<none>"
	}
	return c.image.Summary.Tag
}

func (c *imageContext) Platform() string {
	return platforms.Format(c.image.Summary.Platform)
}

func (c *imageContext) Size() string {
	return units.HumanSizeWithPrecision(float64(c.image.Summary.Size), 3)
}

func (c *imageContext) Created() string {
	if c.image.Summary.Created == nil {
		return "N/A"
	}
	return units.HumanDuration(time.Now().UTC().Sub(*c.image.Summary.Created)) + " ago"
}

func (c *imageContext) CreatedAt() string {
	if c.image.Summary.Created == nil {
		return "N/A"
	}
	return c.image.Summary.Created.Format(time.RFC3339Nano)
}

func (c *imageContext) LastTagTime() string {
	return c.image.Summary.LastTagTime.Format(time.RFC3339Nano)
}
