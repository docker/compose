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

package tracing

import (
	"context"

	"github.com/docker/cli/cli/command"
	_ "github.com/moby/buildkit/util/tracing/childprocess" //nolint:blank-imports // initializes BuildKit tracing child process propagation.
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// ShutdownFunc flushes and stops an OTEL exporter.
type ShutdownFunc func(ctx context.Context) error

func InitTracing(dockerCli command.Cli) (ShutdownFunc, error) {
	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return InitProvider(dockerCli)
}

func InitProvider(dockerCli command.Cli) (ShutdownFunc, error) {
	return func(ctx context.Context) error {
		return nil
	}, nil
}
