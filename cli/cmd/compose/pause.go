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

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/progress"
)

type pauseOptions struct {
	*projectOptions
}

func pauseCommand(p *projectOptions) *cobra.Command {
	opts := pauseOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pause [SERVICE...]",
		Short: "pause services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPause(cmd.Context(), opts, args)
		},
	}
	return cmd
}

func runPause(ctx context.Context, opts pauseOptions, services []string) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Pause(ctx, project)
	})
	return err
}

type unpauseOptions struct {
	*projectOptions
}

func unpauseCommand(p *projectOptions) *cobra.Command {
	opts := unpauseOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "unpause [SERVICE...]",
		Short: "unpause services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnPause(cmd.Context(), opts, args)
		},
	}
	return cmd
}

func runUnPause(ctx context.Context, opts unpauseOptions, services []string) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().UnPause(ctx, project)
	})
	return err
}
