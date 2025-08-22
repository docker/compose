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
	"errors"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type MuxExporter struct {
	exporters []sdktrace.SpanExporter
}

func (m MuxExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	var (
		wg    sync.WaitGroup
		errMu sync.Mutex
		errs  = make([]error, 0, len(m.exporters))
	)

	for _, exporter := range m.exporters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exporter.ExportSpans(ctx, spans); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (m MuxExporter) Shutdown(ctx context.Context) error {
	var (
		wg    sync.WaitGroup
		errMu sync.Mutex
		errs  = make([]error, 0, len(m.exporters))
	)

	for _, exporter := range m.exporters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exporter.Shutdown(ctx); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
