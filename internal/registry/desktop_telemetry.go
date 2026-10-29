/*
   Copyright 2026 Docker Compose CLI authors

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

package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/acarl005/stripansi"
	seclient "github.com/docker/secrets-engine/client"
	"github.com/docker/secrets-engine/client/dockerhub"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.19.0"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry for Docker Desktop secrets engine lookups. Usernames, emails and
// tokens are never recorded, and home directories are stripped from errors.
const spanPrefix = "secrets-engine/"

const (
	// attrEnvironment is the Docker Hub environment, e.g. "Docker Hub".
	attrEnvironment = attribute.Key("secrets_engine.environment")
	// attrResult is "success" or "error".
	attrResult = attribute.Key("secrets_engine.result")
	// attrErrorReason is a stable, low-cardinality failure category; see errorReason.
	attrErrorReason = attribute.Key("secrets_engine.error.reason")
	// attrSessionUnavailable is true when the failure only means there is no
	// session to use (Desktop not running, nobody signed in, ...), as opposed
	// to an unexpected error.
	attrSessionUnavailable = attribute.Key("secrets_engine.session_unavailable")
	// attrUsernameFromProfile is true when the session lacked a username and
	// the profile had to be fetched.
	attrUsernameFromProfile = attribute.Key("secrets_engine.username_from_profile")
	// attrCache is one of cacheHit, cacheMiss or cacheRetryBackoff.
	attrCache = attribute.Key("secrets_engine.cache")
	// attrCancelCause is context.Cause of a canceled lookup.
	attrCancelCause = attribute.Key("secrets_engine.cancel.cause")
)

const (
	cacheHit          = "hit"
	cacheMiss         = "miss"
	cacheRetryBackoff = "retry_backoff"

	resultSuccess = "success"
	resultError   = "error"
	// resultSkipped means the secrets engine was not asked because a recent
	// lookup failed.
	resultSkipped = "skipped"
	// resultCanceled means the caller stopped waiting, e.g. the user pressed
	// Ctrl-C; see attrCancelCause.
	resultCanceled = "canceled"
)

// Values of attrErrorReason.
const (
	reasonConnectNotRunning       = "connect_not_running"
	reasonConnectPermissionDenied = "connect_permission_denied"
	reasonConnectTimeout          = "connect_timeout"
	reasonConnectUnknown          = "connect_unknown"
	reasonAccessDenied            = "access_denied"
	reasonNoDefaultProfile        = "no_default_profile"
	reasonNoSession               = "no_session"
	reasonSessionExpired          = "session_expired"
	reasonNoUsername              = "no_username"
	reasonTimeout                 = "timeout"
	reasonCanceled                = "canceled"
	reasonOther                   = "other"
)

// traceCall runs one secrets engine client call in a child span.
func traceCall[T any](ctx context.Context, tracer trace.Tracer, name string, call func(context.Context) (T, error)) (T, error) {
	ctx, span := tracer.Start(ctx, spanPrefix+name, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	v, err := call(ctx)
	recordResult(ctx, span, err)
	return v, err
}

// recordResult records err, the result of a call made with ctx. A call that
// failed because its caller stopped waiting is recorded as canceled, with the
// cause, rather than as a secrets engine failure.
func recordResult(ctx context.Context, span trace.Span, err error) {
	if err == nil {
		span.SetAttributes(attrResult.String(resultSuccess))
		span.SetStatus(codes.Ok, "")
		return
	}
	reason := errorReason(err)
	if ctx.Err() != nil {
		cause := context.Cause(ctx)
		if !errors.Is(cause, errLookupTimeout) {
			span.SetAttributes(
				attrResult.String(resultCanceled),
				attrErrorReason.String(reasonCanceled),
				attrCancelCause.String(errorMessage(cause)),
			)
			return
		}
		reason = reasonTimeout
	}
	msg := errorMessage(err)
	span.SetAttributes(
		attrResult.String(resultError),
		attrErrorReason.String(reason),
		attrSessionUnavailable.Bool(isSessionUnavailable(err)),
	)
	span.AddEvent(semconv.ExceptionEventName, trace.WithAttributes(
		semconv.ExceptionType(fmt.Sprintf("%T", err)),
		semconv.ExceptionMessage(msg),
	))
	span.SetStatus(codes.Error, msg)
}

// errorMessage returns err's message without ANSI escapes and with the user's
// home directory, which names their OS account, replaced by "~". Socket paths
// in connect errors live under it.
func errorMessage(err error) string {
	msg := stripansi.Strip(err.Error())
	if home, homeErr := os.UserHomeDir(); homeErr == nil && len(home) > 1 {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return msg
}

// errorReason maps a lookup error to a value of attrErrorReason.
func errorReason(err error) string {
	if connErr, ok := errors.AsType[*seclient.ConnectError](err); ok {
		switch connErr.Reason {
		case seclient.ReasonNotRunning:
			return reasonConnectNotRunning
		case seclient.ReasonPermissionDenied:
			return reasonConnectPermissionDenied
		case seclient.ReasonTimeout:
			return reasonConnectTimeout
		default:
			return reasonConnectUnknown
		}
	}
	switch {
	case errors.Is(err, seclient.ErrAccessDenied):
		return reasonAccessDenied
	// ErrNoDefaultProfile wraps ErrNoSession, so it must be checked first.
	case errors.Is(err, dockerhub.ErrNoDefaultProfile):
		return reasonNoDefaultProfile
	case errors.Is(err, dockerhub.ErrNoSession):
		return reasonNoSession
	case errors.Is(err, errSessionExpired):
		return reasonSessionExpired
	case errors.Is(err, errNoUsername):
		return reasonNoUsername
	case errors.Is(err, errLookupTimeout), errors.Is(err, context.DeadlineExceeded):
		return reasonTimeout
	case errors.Is(err, context.Canceled):
		return reasonCanceled
	default:
		return reasonOther
	}
}
