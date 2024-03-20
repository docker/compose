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
	"strconv"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/internal"
	"github.com/hashicorp/go-multierror"
	"github.com/moby/buildkit/util/tracing/detect"
	_ "github.com/moby/buildkit/util/tracing/detect/delegated" //nolint:blank-imports
	_ "github.com/moby/buildkit/util/tracing/env"              //nolint:blank-imports
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.19.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

func init() {
	detect.ServiceName = "compose"
	// do not log tracing errors to stdio
	otel.SetErrorHandler(skipErrors{})
}

var Tracer = otel.Tracer("compose")

// ShutdownFunc flushes and stops an OTEL exporter.
type ShutdownFunc func(ctx context.Context) error

// envMap is a convenience type for OS environment variables.
type envMap map[string]string

// Initialize configures tracing & metering for the application.
//
// Tracing supports exporting to a user-defined OTLP/gRPC endpoint. Additionally, if
// the active Docker CLI context includes a _local_ OTLP endpoint, traces will also
// be exported there.
//
// Metering currently only supports exporting to the destination configured in the
// Docker CLI context, and metrics are reported exactly once as part of the shutdown.
func Initialize(ctx context.Context, dockerCli command.Cli) (ShutdownFunc, error) {
	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if v, _ := strconv.ParseBool(os.Getenv("COMPOSE_EXPERIMENTAL_OTEL")); !v {
		return nil, nil
	}

	res, err := createResource(ctx, resource.WithAttributes(
		attribute.String("docker.context", dockerCli.CurrentContext()),
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	shutdown := startProviders(ctx, dockerCli, res)
	return shutdown, nil
}

// createTraceProvider creates a trace.TracerProvider based on OS environment and Docker CLI context config.
func createTraceProvider(ctx context.Context, res *resource.Resource, otelEnv envMap, dockerOTLPConn *grpc.ClientConn) (trace.TracerProvider, ShutdownFunc, error) {
	var errs []error
	var traceExporters []sdktrace.SpanExporter

	// configure a client from OTEL_ env vars set by user
	envClient := userTraceClient(otelEnv)
	if envClient != nil {
		if envExporter, err := otlptrace.New(ctx, envClient); err != nil {
			errs = append(errs, err)
		} else if envExporter != nil {
			traceExporters = append(traceExporters, envExporter)
		}
	}

	// configure a client from the Docker CLI context metadata
	if dockerOTLPConn != nil {
		dockerExporter, err := withoutOTelEnv(otelEnv, func() (sdktrace.SpanExporter, error) {
			client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(dockerOTLPConn))
			return otlptrace.New(ctx, client)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("creating Docker traces exporter: %w", err))
		} else if dockerExporter != nil {
			traceExporters = append(traceExporters, dockerExporter)
		}
	}

	if len(errs) != 0 {
		return nil, nil, errors.Join(errs...)
	}

	muxExporter := MuxExporter{exporters: traceExporters}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(muxExporter),
	)
	return tracerProvider, tracerProvider.Shutdown, nil
}

// createMeterProvider creates a metric.MeterProvider based on OS environment and Docker CLI context config.
func createMeterProvider(ctx context.Context, res *resource.Resource, otelEnv envMap, dockerOTLPConn *grpc.ClientConn) (metric.MeterProvider, ShutdownFunc, error) {
	if dockerOTLPConn == nil {
		// TODO(milas): support custom OTLP metrics endpoints as well
		return nil, nil, nil
	}

	meterReader := sdkmetric.NewManualReader(
		sdkmetric.WithTemporalitySelector(func(_ sdkmetric.InstrumentKind) metricdata.Temporality {
			return metricdata.DeltaTemporality
		}),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(meterReader),
	)

	dockerExporter, err := withoutOTelEnv(otelEnv, func() (sdkmetric.Exporter, error) {
		return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(dockerOTLPConn))
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating Docker metrics exporter: %w", err)
	}

	var shutdown ShutdownFunc = func(ctx context.Context) error {
		// this must run serially, so collect errors as we go
		var errs []error

		// send the report
		var rm metricdata.ResourceMetrics
		if err := meterReader.Collect(ctx, &rm); err != nil {
			errs = append(errs, err)
		} else {
			errs = append(errs, dockerExporter.Export(ctx, &rm))
		}

		// release any remaining resources
		errs = append(errs, dockerExporter.Shutdown(ctx))
		errs = append(errs, meterProvider.Shutdown(ctx))

		return errors.Join(errs...)
	}

	return meterProvider, shutdown, nil
}

// createResource creates the resource.Resource for Compose with common metadata attached.
func createResource(ctx context.Context, opts ...resource.Option) (*resource.Resource, error) {
	opts = append(opts, resource.WithAttributes(
		semconv.ServiceName("compose"),
		semconv.ServiceVersion(internal.Version),
	))
	res, err := resource.New(ctx, opts...)
	return res, err
}

// startProviders creates and starts both tracing & meter providers if configured.
//
// Either nil is returned, in which case no cleanup is required, or a non-nil ShutdownFunc that
// should be called before process exit to flush data.
func startProviders(ctx context.Context, dockerCli command.Cli, res *resource.Resource) ShutdownFunc {
	otelEnv := readOTelEnv()

	// attempt to extract an OTEL config from the Docker context to enable
	// automatic integration with Docker Desktop
	cfg, err := ConfigFromDockerContext(dockerCli.ContextStore(), dockerCli.CurrentContext())
	if err != nil {
		logrus.Debugf("Failed to load otel config from Docker context metadata: %v", err)
	}

	dockerOTLPConn, err := grpcConnection(ctx, cfg)
	if err != nil {
		logrus.Debugf("Failed to connect to Docker OTLP endpoint: %v", err)
	}

	tracerProvider, traceShutdown, err := createTraceProvider(ctx, res, otelEnv, dockerOTLPConn)
	if err != nil {
		logrus.Debugf("Failed to create trace provider: %v", err)
	} else if tracerProvider != nil {
		otel.SetTracerProvider(tracerProvider)
	}

	meterProvider, metricShutdown, err := createMeterProvider(ctx, res, otelEnv, dockerOTLPConn)
	if err != nil {
		logrus.Debugf("Failed to create meter provider: %v", err)
	} else if meterProvider != nil {
		otel.SetMeterProvider(meterProvider)
	}

	if traceShutdown == nil && metricShutdown == nil {
		// nothing to shut down
		return nil
	}

	// shutdown flushes data and shut down the providers/exporters.
	var shutdown ShutdownFunc = func(ctx context.Context) error {
		defer func() {
			if dockerOTLPConn != nil {
				// must manually clean up the connection AFTER the shutdowns
				// are finished (since they use the connection to flush data)
				_ = dockerOTLPConn.Close()
			}
		}()

		var eg multierror.Group
		if traceShutdown != nil {
			eg.Go(func() error {
				return traceShutdown(ctx)
			})
		}
		if metricShutdown != nil {
			eg.Go(func() error {
				return metricShutdown(ctx)
			})
		}
		return eg.Wait()
	}
	return shutdown
}

// readOTelEnv returns a map of all environment variables that start with `OTEL_`.
//
// See withoutOtelEnv for how this is used to work around a work in the OTel SDK.
func readOTelEnv() envMap {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "OTEL_") {
			env[k] = v
		}
	}
	return env
}

// userTraceClient creates a gRPC OTLP client based on OS environment
// variables.
//
// https://opentelemetry.io/docs/concepts/sdk-configuration/otlp-exporter-configuration/
func userTraceClient(otelEnv envMap) otlptrace.Client {
	for k := range otelEnv {
		if strings.HasPrefix(k, "OTEL_") && strings.HasSuffix(k, "ENDPOINT") {
			// TODO(milas): switch to autoexport to support more than gRPC
			return otlptracegrpc.NewClient()
		}
	}
	return nil
}
