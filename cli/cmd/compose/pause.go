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

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"
)

type pauseOptions struct {
	*projectOptions
}

func pauseCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := pauseOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pause [SERVICE...]",
		Short: "pause services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPause(ctx, backend, opts, args)
		}),
	}
	return cmd
}

func runPause(ctx context.Context, backend compose.Service, opts pauseOptions, services []string) error {
	project, err := opts.toProjectName()
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", backend.Pause(ctx, project, compose.PauseOptions{
			Services: services,
		})
	})
	return err
}

type unpauseOptions struct {
	*projectOptions
}

func unpauseCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := unpauseOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "unpause [SERVICE...]",
		Short: "unpause services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runUnPause(ctx, backend, opts, args)
		}),
	}
	return cmd
}

func runUnPause(ctx context.Context, backend compose.Service, opts unpauseOptions, services []string) error {
	project, err := opts.toProjectName()
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", backend.UnPause(ctx, project, compose.PauseOptions{
			Services: services,
		})
	})
	return err
}
