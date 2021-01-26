// +build kube

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

package kube

import (
	"context"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/kube/charts"
)

// NewComposeService create a kubernetes implementation of the compose.Service API
func NewComposeService(ctx store.KubeContext) (compose.Service, error) {
	chartsAPI, err := charts.NewSDK(ctx)
	if err != nil {
		return nil, err
	}
	return &composeService{
		ctx: ctx,
		sdk: chartsAPI,
	}, nil
}

type composeService struct {
	ctx store.KubeContext
	sdk charts.SDK
}

// Up executes the equivalent to a `compose up`
func (s *composeService) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	return s.sdk.Install(project)
}

// Down executes the equivalent to a `compose down`
func (s *composeService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	return s.sdk.Uninstall(projectName)
}

// List executes the equivalent to a `docker stack ls`
func (s *composeService) List(ctx context.Context) ([]compose.Stack, error) {
	return s.sdk.List()
}

// Build executes the equivalent to a `compose build`
func (s *composeService) Build(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

// Push executes the equivalent ot a `compose push`
func (s *composeService) Push(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

// Pull executes the equivalent of a `compose pull`
func (s *composeService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

// Create executes the equivalent to a `compose create`
func (s *composeService) Create(ctx context.Context, project *types.Project, opts compose.CreateOptions) error {
	return errdefs.ErrNotImplemented
}

// Start executes the equivalent to a `compose start`
func (s *composeService) Start(ctx context.Context, project *types.Project, consumer compose.LogConsumer) error {
	return errdefs.ErrNotImplemented
}

// Stop executes the equivalent to a `compose stop`
func (s *composeService) Stop(ctx context.Context, project *types.Project, consumer compose.LogConsumer) error {
	return errdefs.ErrNotImplemented
}

// Logs executes the equivalent to a `compose logs`
func (s *composeService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	return errdefs.ErrNotImplemented
}

// Ps executes the equivalent to a `compose ps`
func (s *composeService) Ps(ctx context.Context, projectName string) ([]compose.ContainerSummary, error) {
	return nil, errdefs.ErrNotImplemented
}

// Convert translate compose model into backend's native format
func (s *composeService) Convert(ctx context.Context, project *types.Project, options compose.ConvertOptions) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}

// RunOneOffContainer creates a service oneoff container and starts its dependencies
func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) error {
	return errdefs.ErrNotImplemented
}
