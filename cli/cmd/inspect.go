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

package cmd

import (
	"context"
	"fmt"

	"github.com/docker/compose-cli/cmd/formatter"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/containers"
)

// InspectCommand inspects into containers
func InspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect containers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd.Context(), args[0])
		},
	}

	return cmd
}

func runInspect(ctx context.Context, id string) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	container, err := c.ContainerService().Inspect(ctx, id)
	if err != nil {
		return err
	}

	view := getInspectView(container)

	j, err := formatter.ToStandardJSON(view)
	if err != nil {
		return err
	}
	fmt.Print(j)

	return nil
}

// ContainerInspectView inspect view
type ContainerInspectView struct {
	ID          string
	Status      string
	Image       string
	Command     string                    `json:",omitempty"`
	HostConfig  *containers.HostConfig    `json:",omitempty"`
	Ports       []containers.Port         `json:",omitempty"`
	Config      *containers.RuntimeConfig `json:",omitempty"`
	Platform    string
	Healthcheck *containerInspectHealthcheck `json:",omitempty"`
}

type containerInspectHealthcheck struct {
	// Test is the command to be run to check the health of the container
	Test []string `json:",omitempty"`
	// Interval is the period in between the checks
	Interval *types.Duration `json:",omitempty"`
	// Retries is the number of attempts before declaring the container as healthy or unhealthy
	Retries *int `json:",omitempty"`
	// StartPeriod is the start delay before starting the checks
	StartPeriod *types.Duration `json:",omitempty"`
	// Timeout is the timeout in between checks
	Timeout *types.Duration `json:",omitempty"`
}

func getInspectView(container containers.Container) ContainerInspectView {
	var (
		healthcheck *containerInspectHealthcheck
		test        []string
		retries     *int
		ports       []containers.Port
	)

	if len(container.Ports) > 0 {
		ports = container.Ports
	}
	if !container.Healthcheck.Disable && len(container.Healthcheck.Test) > 0 {
		test = container.Healthcheck.Test
		if container.Healthcheck.Retries != 0 {
			retries = to.IntPtr(container.Healthcheck.Retries)
		}
		getDurationPtr := func(d types.Duration) *types.Duration {
			if d == types.Duration(0) {
				return nil
			}
			return &d
		}

		healthcheck = &containerInspectHealthcheck{
			Test:        test,
			Retries:     retries,
			Interval:    getDurationPtr(container.Healthcheck.Interval),
			StartPeriod: getDurationPtr(container.Healthcheck.StartPeriod),
			Timeout:     getDurationPtr(container.Healthcheck.Timeout),
		}
	}

	return ContainerInspectView{
		ID:      container.ID,
		Status:  container.Status,
		Image:   container.Image,
		Command: container.Command,

		Config:      container.Config,
		HostConfig:  container.HostConfig,
		Ports:       ports,
		Platform:    container.Platform,
		Healthcheck: healthcheck,
	}
}
