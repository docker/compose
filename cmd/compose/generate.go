/*
   Copyright 2023 Docker Compose CLI authors

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
	"os"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type generateOptions struct {
	*ProjectOptions
	Format string
}

func generateCommand(p *ProjectOptions, backend api.Service) *cobra.Command {
	opts := generateOptions{
		ProjectOptions: p,
	}

	cmd := &cobra.Command{
		Use:   "generate [OPTIONS] [CONTAINERS...]",
		Short: "EXPERIMENTAL - Generate a Compose file from existing containers",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runGenerate(ctx, backend, opts, args)
		}),
	}

	cmd.Flags().StringVar(&opts.ProjectName, "name", "", "Project name to set in the Compose file")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", "", "Directory to use for the project")
	cmd.Flags().StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")
	return cmd
}

func runGenerate(ctx context.Context, backend api.Service, opts generateOptions, containers []string) error {
	_, _ = fmt.Fprintln(os.Stderr, "generate command is EXPERIMENTAL")
	if len(containers) == 0 {
		return fmt.Errorf("at least one container must be specified")
	}
	project, err := backend.Generate(ctx, api.GenerateOptions{
		Containers:  containers,
		ProjectName: opts.ProjectName,
	})
	if err != nil {
		return err
	}
	var content []byte
	switch opts.Format {
	case "json":
		content, err = project.MarshalJSON()
	case "yaml":
		content, err = project.MarshalYAML()
	default:
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(content))

	return nil
}
