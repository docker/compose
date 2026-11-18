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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/types/image"

	"github.com/docker/compose/v5/internal/devwatch"
	"github.com/docker/compose/v5/pkg/api"
)

// The development loop (develop.watch) lives in internal/devwatch:
// composeService serves it by handing that package the engine it drives.

func (s *composeService) devWatch() *devwatch.Service {
	return devwatch.New(devWatchBackend{engineBackend{s}})
}

func (s *composeService) Watch(ctx context.Context, project *types.Project, options api.WatchOptions) error {
	wait, err := s.watch(ctx, project, options)
	if err != nil {
		return err
	}
	return wait()
}

func (s *composeService) watch(ctx context.Context, project *types.Project, options api.WatchOptions) (func() error, error) {
	return s.devWatch().Watch(ctx, project, options)
}

// devWatchBackend is the engine of a composeService, as internal/devwatch sees it.
type devWatchBackend struct {
	engineBackend
}

func (b devWatchBackend) CLI() command.Cli {
	return b.s.dockerCli
}

func (b devWatchBackend) Build(ctx context.Context, project *types.Project, options api.BuildOptions) (map[string]string, error) {
	return b.s.build(ctx, project, options, nil)
}

func (b devWatchBackend) CreateAndStart(ctx context.Context, project *types.Project, options api.CreateOptions) error {
	return b.s.createAndStart(ctx, project, options)
}

func (b devWatchBackend) Restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	return b.s.restart(ctx, projectName, options)
}

func (b devWatchBackend) RemoveDanglingImages(ctx context.Context, projectName string, keep func(image.Summary) bool) error {
	_, err := b.s.removeDanglingImages(ctx, projectName, keep)
	return err
}
