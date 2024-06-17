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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/compose/v2/internal"
	"go.opentelemetry.io/otel/attribute"

	"github.com/docker/cli/cli/command"
	"github.com/moby/buildkit/util/tracing/detect"
	_ "github.com/moby/buildkit/util/tracing/env" //nolint:blank-imports
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func init() {
	detect.ServiceName = "compose"
	// do not log tracing errors to stdio
	otel.SetErrorHandler(skipErrors{})
}

// OTLPConfig contains the necessary values to initialize an OTLP client
// manually.
//
// This supports a minimal set of options based on what is necessary for
// automatic OTEL configuration from Docker context metadata.
type OTLPConfig struct {
	Endpoint string
}

// ShutdownFunc flushes and stops an OTEL exporter.
type ShutdownFunc func(ctx context.Context) error

// envMap is a convenience type for OS environment variables.
type envMap map[string]string

func InitTracing(dockerCli command.Cli) (ShutdownFunc, error) {
	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return InitProvider(dockerCli)
}

func InitProvider(dockerCli command.Cli) (ShutdownFunc, error) {
	ctx := context.Background()

	var errs []error
	var exporters []sdktrace.SpanExporter

	envClient, otelEnv := traceClientFromEnv()
	if envClient != nil {
		if envExporter, err := otlptrace.New(ctx, envClient); err != nil {
			errs = append(errs, err)
		} else if envExporter != nil {
			exporters = append(exporters, envExporter)
		}
	}

	if dcClient, err := traceClientFromDockerContext(dockerCli, otelEnv); err != nil {
		errs = append(errs, err)
	} else if dcClient != nil {
		if dcExporter, err := otlptrace.New(ctx, dcClient); err != nil {
			errs = append(errs, err)
		} else if dcExporter != nil {
			exporters = append(exporters, dcExporter)
		}
	}
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName("compose"),
			semconv.ServiceVersion(internal.Version),
			attribute.String("docker.context", dockerCli.CurrentContext()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	muxExporter := MuxExporter{exporters: exporters}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(muxExporter),
	)
	otel.SetTracerProvider(tracerProvider)

	// Shutdown will flush any remaining spans and shut down the exporter.
	return tracerProvider.Shutdown, nil
}

// traceClientFromEnv creates a GRPC OTLP client based on OS environment
// variables.
//
// https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/
func traceClientFromEnv() (otlptrace.Client, envMap) {
	hasOtelEndpointInEnv := false
	otelEnv := make(map[string]string)
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "OTEL_") {
			otelEnv[k] = v
			if strings.HasSuffix(k, "ENDPOINT") {
				hasOtelEndpointInEnv = true
			}
		}
	}

	if !hasOtelEndpointInEnv {
		return nil, nil
	}

	client := otlptracegrpc.NewClient()
	return client, otelEnv
}
