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

// Package containerops implements the operations that act from the outside on
// the container of a service: exec, cp, export and commit.
package containerops

import (
	"context"

	"github.com/docker/cli/cli/command"
	"github.com/moby/moby/api/types/container"

	"github.com/docker/compose/v5/internal/concurrency"
	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/pkg/api"
)

// Backend is what the operations need from the engine they act on.
//
// The terminal reaches them through CLI: stdin, stdout and the TTY all come
// from the Docker CLI's streams, the one seam through which an operation that
// talks to the user does so (exec hands the whole CLI to the Docker CLI's own
// implementation).
type Backend interface {
	engine.Backend
	// Events receives the progress of the operations.
	Events() api.EventProcessor
	// DryRun tells whether the operations only report what they would do.
	DryRun() bool
	// CLI is the Docker CLI Compose runs in, and the owner of the terminal streams.
	CLI() command.Cli
}

// Service runs the container operations on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service acting on the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}

// forEachContainerConcurrent runs fn for every container concurrently, at most
// maxConcurrency at a time (0 meaning no bound), and waits for all of them.
func forEachContainerConcurrent(ctx context.Context, maxConcurrency int, containers []container.Summary, fn func(context.Context, container.Summary) error) error {
	eg, ctx := concurrency.NewErrgroup(ctx, maxConcurrency)
	for _, ctr := range containers {
		eg.Go(func() error {
			return fn(ctx, ctr)
		})
	}
	return eg.Wait()
}
