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

type tagOptions struct {
	*projectOptions
	composeOptions

	Template           string
	Push               bool
	IgnorePushFailures bool
}

func tagCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := tagOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "tag [SERVICE...]",
		Short: "tag service images",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runTag(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Template, "template", "", "Re-tag images using string template before pushing them"+
		"\n  .i.e --template \"registry.lan/repo/{{ .ServiceName }}:v1.2.4\""+
		"\n Possible template vars are: ServiceName, ProjectName, ServicesCount, ServicesToBuildCount")
	flags.BoolVar(&opts.Push, "push", false, "Push the new tag directly")
	flags.BoolVar(&opts.IgnorePushFailures, "ignore-push-failures", false, "Push what it can and ignores images with push failures")
	return cmd
}

func runTag(ctx context.Context, backend api.Service, opts tagOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	return backend.Tag(ctx, project, api.TagOptions{
		Template: opts.Template,
		Push:     opts.Push,
	})
}
