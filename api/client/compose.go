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

package client

import (
	"context"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/pkg/api"
)

type composeService struct {
}

func (c *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Pull(ctx context.Context, project *types.Project, options api.PullOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Create(ctx context.Context, project *types.Project, opts api.CreateOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Start(ctx context.Context, project *types.Project, options api.StartOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Restart(ctx context.Context, project *types.Project, options api.RestartOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Stop(ctx context.Context, project *types.Project, options api.StopOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Up(context.Context, *types.Project, api.UpOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Down(context.Context, string, api.DownOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Logs(context.Context, string, api.LogConsumer, api.LogOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Ps(context.Context, string, api.PsOptions) ([]api.ContainerSummary, error) {
	return nil, api.ErrNotImplemented
}

func (c *composeService) List(context.Context, api.ListOptions) ([]api.Stack, error) {
	return nil, api.ErrNotImplemented
}

func (c *composeService) Convert(context.Context, *types.Project, api.ConvertOptions) ([]byte, error) {
	return nil, api.ErrNotImplemented
}

func (c *composeService) Kill(ctx context.Context, project *types.Project, options api.KillOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	return 0, api.ErrNotImplemented
}

func (c *composeService) Remove(ctx context.Context, project *types.Project, options api.RemoveOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Exec(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	return 0, api.ErrNotImplemented
}

func (c *composeService) Copy(ctx context.Context, project *types.Project, opts api.CopyOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Pause(ctx context.Context, project string, options api.PauseOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) UnPause(ctx context.Context, project string, options api.PauseOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Top(ctx context.Context, projectName string, services []string) ([]api.ContainerProcSummary, error) {
	return nil, api.ErrNotImplemented
}

func (c *composeService) Events(ctx context.Context, project string, options api.EventsOptions) error {
	return api.ErrNotImplemented
}

func (c *composeService) Port(ctx context.Context, project string, service string, port int, options api.PortOptions) (string, int, error) {
	return "", 0, api.ErrNotImplemented
}

func (c *composeService) Images(ctx context.Context, projectName string, options api.ImagesOptions) ([]api.ImageSummary, error) {
	return nil, api.ErrNotImplemented
}
