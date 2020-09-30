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
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/formatter"
)

func psCommand() *cobra.Command {
	opts := composeOptions{}
	psCmd := &cobra.Command{
		Use: "ps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPs(cmd.Context(), opts)
		},
	}
	psCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	psCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	addComposeCommonFlags(psCmd.Flags(), &opts)
	return psCmd
}

func runPs(ctx context.Context, opts composeOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	serviceList, err := c.ComposeService().Ps(ctx, projectName)
	if err != nil {
		return err
	}

	view := viewFromServiceStatusList(serviceList)
	return formatter.Print(view, opts.Format, os.Stdout,
		func(w io.Writer) {
			for _, service := range view {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", service.ID, service.Name, service.Replicas, service.Desired, strings.Join(service.Ports, ", "))
			}
		},
		"ID", "NAME", "REPLICAS", "PORTS")
}

type serviceStatusView struct {
	ID       string
	Name     string
	Replicas int
	Desired  int
	Ports    []string
}

func viewFromServiceStatusList(serviceStatusList []compose.ServiceStatus) []serviceStatusView {
	retList := make([]serviceStatusView, len(serviceStatusList))
	for i, s := range serviceStatusList {
		retList[i] = serviceStatusView{
			ID:       s.ID,
			Name:     s.Name,
			Replicas: s.Replicas,
			Desired:  s.Desired,
			Ports:    s.Ports,
		}
	}
	return retList
}
