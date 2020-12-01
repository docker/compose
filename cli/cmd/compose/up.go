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
	"github.com/docker/compose-cli/progress"
	"os"

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/context/store"
)

func upCommand(contextType string) *cobra.Command {
	opts := composeOptions{}
	upCmd := &cobra.Command{
		Use: "up [SERVICE...]",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd.Context(), opts, args)
		},
	}
	upCmd.Flags().StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	upCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	upCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	upCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")
	upCmd.Flags().BoolVarP(&opts.Detach, "detach", "d", false, " Detached mode: Run containers in the background")

	if contextType == store.AciContextType {
		upCmd.Flags().StringVar(&opts.DomainName, "domainname", "", "Container NIS domain name")
	}

	return upCmd
}

func runUp(ctx context.Context, opts composeOptions, services []string) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	options, err := opts.toProjectOptions()
	if err != nil {
		return err
	}
	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return err
	}
	if opts.DomainName != "" {
		// arbitrarily set the domain name on the first service ; ACI backend will expose the entire project
		project.Services[0].DomainName = opts.DomainName
	}

	err = filter(project, services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Up(ctx, project, opts.Detach, os.Stdout)
	})
	return err
}
