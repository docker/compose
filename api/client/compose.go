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

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/errdefs"

	"github.com/compose-spec/compose-go/types"
)

type composeService struct {
}

func (c *composeService) Build(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Push(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Create(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Start(ctx context.Context, project *types.Project, consumer compose.LogConsumer) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Up(context.Context, *types.Project, compose.UpOptions) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Down(context.Context, string, compose.DownOptions) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Logs(context.Context, string, compose.LogConsumer, compose.LogOptions) error {
	return errdefs.ErrNotImplemented
}

func (c *composeService) Ps(context.Context, string) ([]compose.ContainerSummary, error) {
	return nil, errdefs.ErrNotImplemented
}

func (c *composeService) List(context.Context, string) ([]compose.Stack, error) {
	return nil, errdefs.ErrNotImplemented
}

func (c *composeService) Convert(context.Context, *types.Project, compose.ConvertOptions) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}
