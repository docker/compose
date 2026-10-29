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

	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/internal/containerops"
	"github.com/docker/compose/v5/pkg/api"
)

// exec, cp, export and commit live in internal/containerops: composeService
// serves them by handing that package the engine they act on.

func (s *composeService) containerOps() *containerops.Service {
	return containerops.New(containerOpsBackend{engineBackend{s}})
}

func (s *composeService) Exec(ctx context.Context, projectName string, options api.RunOptions) (int, error) {
	return s.containerOps().Exec(ctx, projectName, options)
}

func (s *composeService) Copy(ctx context.Context, projectName string, options api.CopyOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.containerOps().Copy(ctx, projectName, options)
	}, "copy", s.events)
}

func (s *composeService) Export(ctx context.Context, projectName string, options api.ExportOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.containerOps().Export(ctx, projectName, options)
	}, "export", s.events)
}

func (s *composeService) Commit(ctx context.Context, projectName string, options api.CommitOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.containerOps().Commit(ctx, projectName, options)
	}, "commit", s.events)
}

// containerOpsBackend is the engine of a composeService, as internal/containerops sees it.
type containerOpsBackend struct {
	engineBackend
}

func (b containerOpsBackend) Events() api.EventProcessor {
	return b.s.events
}

func (b containerOpsBackend) DryRun() bool {
	return b.s.dryRun
}

func (b containerOpsBackend) CLI() command.Cli {
	return b.s.dockerCli
}
