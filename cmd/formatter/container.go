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

package formatter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/cli/cli/command/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-units"
)

const (
	defaultContainerTableFormat = "table {{.Name}}\t{{.Image}}\t{{.Command}}\t{{.Service}}\t{{.RunningFor}}\t{{.Status}}\t{{.Ports}}"

	nameHeader       = "NAME"
	projectHeader    = "PROJECT"
	serviceHeader    = "SERVICE"
	commandHeader    = "COMMAND"
	runningForHeader = "CREATED"
	mountsHeader     = "MOUNTS"
	localVolumes     = "LOCAL VOLUMES"
	networksHeader   = "NETWORKS"
)

// NewContainerFormat returns a Format for rendering using a Context
func NewContainerFormat(source string, quiet bool, size bool) formatter.Format {
	switch source {
	case formatter.TableFormatKey, "": // table formatting is the default if none is set.
		if quiet {
			return formatter.DefaultQuietFormat
		}
		format := defaultContainerTableFormat
		if size {
			format += `\t{{.Size}}`
		}
		return formatter.Format(format)
	case formatter.RawFormatKey:
		if quiet {
			return `container_id: {{.ID}}`
		}
		format := `container_id: {{.ID}}
image: {{.Image}}
command: {{.Command}}
created_at: {{.CreatedAt}}
state: {{- pad .State 1 0}}
status: {{- pad .Status 1 0}}
names: {{.Names}}
labels: {{- pad .Labels 1 0}}
ports: {{- pad .Ports 1 0}}
`
		if size {
			format += `size: {{.Size}}\n`
		}
		return formatter.Format(format)
	default: // custom format
		if quiet {
			return formatter.DefaultQuietFormat
		}
		return formatter.Format(source)
	}
}

// ContainerWrite renders the context for a list of containers
func ContainerWrite(ctx formatter.Context, containers []api.ContainerSummary) error {
	render := func(format func(subContext formatter.SubContext) error) error {
		for _, container := range containers {
			err := format(&ContainerContext{trunc: ctx.Trunc, c: container})
			if err != nil {
				return err
			}
		}
		return nil
	}
	return ctx.Write(NewContainerContext(), render)
}

// ContainerContext is a struct used for rendering a list of containers in a Go template.
type ContainerContext struct {
	formatter.HeaderContext
	trunc bool
	c     api.ContainerSummary

	// FieldsUsed is used in the pre-processing step to detect which fields are
	// used in the template. It's currently only used to detect use of the .Size
	// field which (if used) automatically sets the '--size' option when making
	// the API call.
	FieldsUsed map[string]interface{}
}

// NewContainerContext creates a new context for rendering containers
func NewContainerContext() *ContainerContext {
	containerCtx := ContainerContext{}
	containerCtx.Header = formatter.SubHeaderContext{
		"ID":         formatter.ContainerIDHeader,
		"Name":       nameHeader,
		"Project":    projectHeader,
		"Service":    serviceHeader,
		"Image":      formatter.ImageHeader,
		"Command":    commandHeader,
		"CreatedAt":  formatter.CreatedAtHeader,
		"RunningFor": runningForHeader,
		"Ports":      formatter.PortsHeader,
		"State":      formatter.StateHeader,
		"Status":     formatter.StatusHeader,
		"Size":       formatter.SizeHeader,
		"Labels":     formatter.LabelsHeader,
	}
	return &containerCtx
}

// MarshalJSON makes ContainerContext implement json.Marshaler
func (c *ContainerContext) MarshalJSON() ([]byte, error) {
	return formatter.MarshalJSON(c)
}

// ID returns the container's ID as a string. Depending on the `--no-trunc`
// option being set, the full or truncated ID is returned.
func (c *ContainerContext) ID() string {
	if c.trunc {
		return stringid.TruncateID(c.c.ID)
	}
	return c.c.ID
}

func (c *ContainerContext) Name() string {
	return c.c.Name
}

// Names returns a comma-separated string of the container's names, with their
// slash (/) prefix stripped. Additional names for the container (related to the
// legacy `--link` feature) are omitted.
func (c *ContainerContext) Names() string {
	names := formatter.StripNamePrefix(c.c.Names)
	if c.trunc {
		for _, name := range names {
			if len(strings.Split(name, "/")) == 1 {
				names = []string{name}
				break
			}
		}
	}
	return strings.Join(names, ",")
}

func (c *ContainerContext) Service() string {
	return c.c.Service
}

func (c *ContainerContext) Project() string {
	return c.c.Project
}

func (c *ContainerContext) Image() string {
	return c.c.Image
}

func (c *ContainerContext) Command() string {
	command := c.c.Command
	if c.trunc {
		command = formatter.Ellipsis(command, 20)
	}
	return strconv.Quote(command)
}

func (c *ContainerContext) CreatedAt() string {
	return time.Unix(c.c.Created, 0).String()
}

func (c *ContainerContext) RunningFor() string {
	createdAt := time.Unix(c.c.Created, 0)
	return units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
}

func (c *ContainerContext) ExitCode() int {
	return c.c.ExitCode
}

func (c *ContainerContext) State() string {
	return c.c.State
}

func (c *ContainerContext) Status() string {
	return c.c.Status
}

func (c *ContainerContext) Health() string {
	return c.c.Health
}

func (c *ContainerContext) Publishers() api.PortPublishers {
	return c.c.Publishers
}

func (c *ContainerContext) Ports() string {
	var ports []types.Port
	for _, publisher := range c.c.Publishers {
		ports = append(ports, types.Port{
			IP:          publisher.URL,
			PrivatePort: publisher.TargetPort,
			PublicPort:  publisher.PublishedPort,
			Type:        publisher.Protocol,
		})
	}
	return formatter.DisplayablePorts(ports)
}

// Labels returns a comma-separated string of labels present on the container.
func (c *ContainerContext) Labels() string {
	if c.c.Labels == nil {
		return ""
	}

	var joinLabels []string
	for k, v := range c.c.Labels {
		joinLabels = append(joinLabels, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(joinLabels, ",")
}

// Label returns the value of the label with the given name or an empty string
// if the given label does not exist.
func (c *ContainerContext) Label(name string) string {
	if c.c.Labels == nil {
		return ""
	}
	return c.c.Labels[name]
}

// Mounts returns a comma-separated string of mount names present on the container.
// If the trunc option is set, names can be truncated (ellipsized).
func (c *ContainerContext) Mounts() string {
	var mounts []string
	for _, name := range c.c.Mounts {
		if c.trunc {
			name = formatter.Ellipsis(name, 15)
		}
		mounts = append(mounts, name)
	}
	return strings.Join(mounts, ",")
}

// LocalVolumes returns the number of volumes using the "local" volume driver.
func (c *ContainerContext) LocalVolumes() string {
	return fmt.Sprintf("%d", c.c.LocalVolumes)
}

// Networks returns a comma-separated string of networks that the container is
// attached to.
func (c *ContainerContext) Networks() string {
	return strings.Join(c.c.Networks, ",")
}

// Size returns the container's size and virtual size (e.g. "2B (virtual 21.5MB)")
func (c *ContainerContext) Size() string {
	if c.FieldsUsed == nil {
		c.FieldsUsed = map[string]interface{}{}
	}
	c.FieldsUsed["Size"] = struct{}{}
	srw := units.HumanSizeWithPrecision(float64(c.c.SizeRw), 3)
	sv := units.HumanSizeWithPrecision(float64(c.c.SizeRootFs), 3)

	sf := srw
	if c.c.SizeRootFs > 0 {
		sf = fmt.Sprintf("%s (virtual %s)", srw, sv)
	}
	return sf
}
