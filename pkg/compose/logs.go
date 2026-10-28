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
	"github.com/moby/moby/api/types/container"
	"golang.org/x/sync/semaphore"

	"github.com/docker/compose/v5/internal/logstream"
	"github.com/docker/compose/v5/pkg/api"
)

// logs and attach live in internal/logstream: composeService serves them by
// handing that package the engine they stream from.

func (s *composeService) logStream() *logstream.Service {
	return logstream.New(logStreamBackend{engineBackend{s}})
}

func (s *composeService) Logs(ctx context.Context, projectName string, consumer api.LogConsumer, options api.LogOptions) error {
	return s.logStream().Logs(ctx, projectName, consumer, options)
}

func (s *composeService) Attach(ctx context.Context, projectName string, options api.AttachOptions) error {
	return s.logStream().Attach(ctx, projectName, options)
}

// logPrinter watches application containers and collects their logs.
type logPrinter = logstream.Printer

func newLogPrinter(consumer api.LogConsumer) logPrinter {
	return logstream.NewPrinter(consumer)
}

// attach is what `up` attaches to its containers with.
func (s *composeService) attach(ctx context.Context, project *types.Project, listener api.ContainerEventListener, selectedServices []string) (Containers, error) {
	return s.logStream().AttachContainers(ctx, project, listener, selectedServices)
}

func (s *composeService) doAttachContainer(ctx context.Context, service, id, name string, listener api.ContainerEventListener) error {
	return s.logStream().AttachContainer(ctx, service, id, name, listener)
}

func (s *composeService) inspectWithSlot(ctx context.Context, limiter *semaphore.Weighted, id string) (container.InspectResponse, error) {
	return s.logStream().InspectWithSlot(ctx, limiter, id)
}

func (s *composeService) doLogContainer(
	ctx context.Context,
	limiter *semaphore.Weighted,
	consumer api.LogConsumer,
	name string,
	ctr container.InspectResponse,
	options api.LogOptions,
	cur *logstream.Cursor,
) error {
	return s.logStream().DoLogContainer(ctx, limiter, consumer, name, ctr, options, cur)
}

// logStreamBackend is the engine of a composeService, as internal/logstream sees it:
// the base engine, and the Docker CLI whose terminal streams it drives.
type logStreamBackend struct {
	engineBackend
}

func (b logStreamBackend) CLI() command.Cli {
	return b.s.dockerCli
}
