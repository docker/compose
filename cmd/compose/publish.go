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

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type publishOptions struct {
	*ProjectOptions
	composeOptions
	Repository string
}

func publishCommand(p *ProjectOptions, backend api.Service) *cobra.Command {
	opts := pushOptions{
		ProjectOptions: p,
	}
	publishCmd := &cobra.Command{
		Use:   "publish [OPTIONS] [REPOSITORY]",
		Short: "Publish compose application",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPublish(ctx, backend, opts, args[0])
		}),
		Args: cobra.ExactArgs(1),
	}
	return publishCmd
}

func runPublish(ctx context.Context, backend api.Service, opts pushOptions, repository string) error {
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}

	return backend.Publish(ctx, project, repository)
}
