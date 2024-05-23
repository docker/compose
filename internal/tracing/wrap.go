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

	"github.com/acarl005/stripansi"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.19.0"
	"go.opentelemetry.io/otel/trace"
)

// SpanWrapFunc wraps a function that takes a context with a trace.Span, marking the status as codes.Error if the
// wrapped function returns an error.
//
// The context passed to the function is created from the span to ensure correct propagation.
//
// NOTE: This function is nearly identical to SpanWrapFuncForErrGroup, except the latter is designed specially for
// convenience with errgroup.Group due to its prevalence throughout the codebase. The code is duplicated to avoid
// adding even more levels of function wrapping/indirection.
func SpanWrapFunc(spanName string, opts SpanOptions, fn func(ctx context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		ctx, span := otel.Tracer("").Start(ctx, spanName, opts.SpanStartOptions()...)
		defer span.End()

		if err := fn(ctx); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetStatus(codes.Ok, "")
		return nil
	}
}

// SpanWrapFuncForErrGroup wraps a function that takes a context with a trace.Span, marking the status as codes.Error
// if the wrapped function returns an error.
//
// The context passed to the function is created from the span to ensure correct propagation.
//
// NOTE: This function is nearly identical to SpanWrapFunc, except this function is designed specially for
// convenience with errgroup.Group due to its prevalence throughout the codebase. The code is duplicated to avoid
// adding even more levels of function wrapping/indirection.
func SpanWrapFuncForErrGroup(ctx context.Context, spanName string, opts SpanOptions, fn func(ctx context.Context) error) func() error {
	return func() error {
		ctx, span := otel.Tracer("").Start(ctx, spanName, opts.SpanStartOptions()...)
		defer span.End()

		if err := fn(ctx); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.SetStatus(codes.Ok, "")
		return nil
	}
}

// EventWrapFuncForErrGroup invokes a function and records an event, optionally including the returned
// error as the "exception message" on the event.
//
// This is intended for lightweight usage to wrap errgroup.Group calls where a full span is not desired.
func EventWrapFuncForErrGroup(ctx context.Context, eventName string, opts SpanOptions, fn func(ctx context.Context) error) func() error {
	return func() error {
		span := trace.SpanFromContext(ctx)
		eventOpts := opts.EventOptions()

		err := fn(ctx)
		if err != nil {
			eventOpts = append(eventOpts, trace.WithAttributes(semconv.ExceptionMessage(stripansi.Strip(err.Error()))))
		}
		span.AddEvent(eventName, eventOpts...)

		return err
	}
}

func AddAttributeToSpan(ctx context.Context, attr ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attr...)
}
