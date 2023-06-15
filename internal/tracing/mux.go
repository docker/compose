/*
   Copyright 2023 Docker Compose CLI authors

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

	"github.com/hashicorp/go-multierror"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type MuxExporter struct {
	exporters []sdktrace.SpanExporter
}

func (m MuxExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	var eg multierror.Group
	for i := range m.exporters {
		exporter := m.exporters[i]
		eg.Go(func() error {
			return exporter.ExportSpans(ctx, spans)
		})
	}
	return eg.Wait()
}

func (m MuxExporter) Shutdown(ctx context.Context) error {
	var eg multierror.Group
	for i := range m.exporters {
		exporter := m.exporters[i]
		eg.Go(func() error {
			return exporter.Shutdown(ctx)
		})
	}
	return eg.Wait()
}
