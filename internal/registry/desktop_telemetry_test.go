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
	"path/filepath"
	"strings"
	"testing"
	"time"

	seclient "github.com/docker/secrets-engine/client"
	"github.com/docker/secrets-engine/client/dockerhub"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gotest.tools/v3/assert"
)

// recordSpans gives the provider's sessions a tracer that records in memory.
func recordSpans(p *testProvider) *tracetest.SpanRecorder {
	recorder := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("")
	p.hub.tracer = tracer
	p.stagingHub.tracer = tracer
	return recorder
}

func spansByName(spans []sdktrace.ReadOnlySpan) map[string]sdktrace.ReadOnlySpan {
	byName := make(map[string]sdktrace.ReadOnlySpan, len(spans))
	for _, s := range spans {
		byName[s.Name()] = s
	}
	return byName
}

func spanAttr(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func assertAttr(t *testing.T, s sdktrace.ReadOnlySpan, key attribute.Key, want any) {
	t.Helper()
	v, ok := spanAttr(s, key)
	assert.Assert(t, ok, "span %s has no attribute %s", s.Name(), key)
	assert.Equal(t, v.AsInterface(), want, "span %s attribute %s", s.Name(), key)
}

func TestDesktopTelemetry_Success(t *testing.T) {
	p := newTestProvider(t, &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)

	spans := spansByName(recorder.Ended())
	assert.Equal(t, len(spans), 3)
	root := spans["secrets-engine/desktop-session"]
	assert.Assert(t, root != nil)
	assert.Equal(t, root.Status().Code, codes.Ok)
	assertAttr(t, root, attrEnvironment, "Docker Hub")
	assertAttr(t, root, attrCache, cacheMiss)
	assertAttr(t, root, attrResult, "success")
	assertAttr(t, root, attrUsernameFromProfile, false)
	_, hasReason := spanAttr(root, attrErrorReason)
	assert.Assert(t, !hasReason)

	for _, name := range []string{"secrets-engine/connect", "secrets-engine/get-default-session"} {
		child := spans[name]
		assert.Assert(t, child != nil, name)
		assert.Equal(t, child.Parent().SpanID(), root.SpanContext().SpanID(), name)
		assert.Equal(t, child.Status().Code, codes.Ok, name)
		assertAttr(t, child, attrResult, "success")
		assert.Assert(t, !child.EndTime().Before(child.StartTime()), name)
	}
	_, fetchedProfile := spans["secrets-engine/get-default-profile"]
	assert.Assert(t, !fetchedProfile)
}

// TestDesktopTelemetry_SpanNestsUnderProviderContext checks that lookups nest
// under the operation the provider was created for, e.g. the command's span.
func TestDesktopTelemetry_SpanNestsUnderProviderContext(t *testing.T) {
	p := newTestProvider(t, &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}, nil)
	recorder := recordSpans(p)

	ctx, parent := p.hub.tracer.Start(t.Context(), "parent")
	p.ctx = ctx
	_, err := p.GetAuthConfig("docker.io")
	parent.End()
	assert.NilError(t, err)

	spans := spansByName(recorder.Ended())
	root := spans["secrets-engine/desktop-session"]
	assert.Assert(t, root != nil)
	assert.Equal(t, root.Parent().SpanID(), parent.SpanContext().SpanID())
	assert.Equal(t, root.SpanContext().TraceID(), parent.SpanContext().TraceID())
}

func TestDesktopTelemetry_UsernameFromProfile(t *testing.T) {
	p := newTestProvider(t, &fakeHub{
		session: hubSession("", testNow.Add(time.Hour)),
		profile: dockerhub.Profile{Username: "hubuser"},
	}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)

	spans := spansByName(recorder.Ended())
	assertAttr(t, spans["secrets-engine/desktop-session"], attrUsernameFromProfile, true)
	profile := spans["secrets-engine/get-default-profile"]
	assert.Assert(t, profile != nil)
	assert.Equal(t, profile.Status().Code, codes.Ok)
}

func TestDesktopTelemetry_Failures(t *testing.T) {
	future := testNow.Add(time.Hour)
	tests := []struct {
		name        string
		hub         *fakeHub
		connectErr  error
		failedCall  string
		reason      string
		unavailable bool
	}{
		{
			name:       "client cannot be created",
			connectErr: errors.New("no socket path"),
			failedCall: "secrets-engine/connect",
			reason:     reasonOther,
		},
		{
			name:        "secrets engine not running",
			hub:         &fakeHub{sessionErr: connectError(seclient.ReasonNotRunning)},
			failedCall:  "secrets-engine/get-default-session",
			reason:      reasonConnectNotRunning,
			unavailable: true,
		},
		{
			name:        "socket permission denied",
			hub:         &fakeHub{sessionErr: connectError(seclient.ReasonPermissionDenied)},
			failedCall:  "secrets-engine/get-default-session",
			reason:      reasonConnectPermissionDenied,
			unavailable: true,
		},
		{
			name:        "connect timeout",
			hub:         &fakeHub{sessionErr: connectError(seclient.ReasonTimeout)},
			failedCall:  "secrets-engine/get-default-session",
			reason:      reasonConnectTimeout,
			unavailable: true,
		},
		{
			name:        "nobody signed in",
			hub:         &fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile},
			failedCall:  "secrets-engine/get-default-session",
			reason:      reasonNoDefaultProfile,
			unavailable: true,
		},
		{
			name:        "no session",
			hub:         &fakeHub{sessionErr: dockerhub.ErrNoSession},
			failedCall:  "secrets-engine/get-default-session",
			reason:      reasonNoSession,
			unavailable: true,
		},
		{
			name:       "access denied",
			hub:        &fakeHub{sessionErr: seclient.ErrAccessDenied},
			failedCall: "secrets-engine/get-default-session",
			reason:     reasonAccessDenied,
		},
		{
			name:       "request timeout",
			hub:        &fakeHub{sessionErr: fmt.Errorf("get session: %w", context.DeadlineExceeded)},
			failedCall: "secrets-engine/get-default-session",
			reason:     reasonTimeout,
		},
		{
			name:       "profile lookup fails",
			hub:        &fakeHub{session: hubSession("", future), profileErr: errors.New("boom")},
			failedCall: "secrets-engine/get-default-profile",
			reason:     reasonOther,
		},
		{
			name:   "no username",
			hub:    &fakeHub{session: hubSession("", future)},
			reason: reasonNoUsername,
		},
		{
			name:        "expired",
			hub:         &fakeHub{session: hubSession("hubuser", testNow.Add(-time.Minute))},
			reason:      reasonSessionExpired,
			unavailable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var hub hubSessions
			if tc.hub != nil {
				hub = tc.hub
			}
			p := newTestProvider(t, hub, tc.connectErr)
			recorder := recordSpans(p)

			_, err := p.GetAuthConfig("docker.io")
			assert.NilError(t, err)

			spans := spansByName(recorder.Ended())
			root := spans["secrets-engine/desktop-session"]
			assert.Assert(t, root != nil)
			assert.Equal(t, root.Status().Code, codes.Error)
			assert.Assert(t, root.Status().Description != "")
			assert.Equal(t, exceptionMessage(t, root), root.Status().Description)
			assertAttr(t, root, attrResult, "error")
			assertAttr(t, root, attrErrorReason, tc.reason)
			assertAttr(t, root, attrSessionUnavailable, tc.unavailable)

			for name, s := range spans {
				if name == root.Name() {
					continue
				}
				if name == tc.failedCall {
					assert.Equal(t, s.Status().Code, codes.Error, name)
					assertAttr(t, s, attrErrorReason, tc.reason)
				} else {
					assert.Equal(t, s.Status().Code, codes.Ok, name)
				}
			}
		})
	}
}

func spanStrings(s sdktrace.ReadOnlySpan) []string {
	out := []string{s.Name(), s.Status().Description}
	attrs := s.Attributes()
	for _, e := range s.Events() {
		out = append(out, e.Name)
		attrs = append(attrs, e.Attributes...)
	}
	for _, kv := range attrs {
		out = append(out, kv.Value.String())
	}
	return out
}

func assertNotRecorded(t *testing.T, spans []sdktrace.ReadOnlySpan, values ...string) {
	t.Helper()
	for _, s := range spans {
		for _, got := range spanStrings(s) {
			for _, v := range values {
				assert.Assert(t, !strings.Contains(got, v), "span %s records %q in %q", s.Name(), v, got)
			}
		}
	}
}

func TestDesktopTelemetry_DoesNotRecordSessionIdentity(t *testing.T) {
	session := hubSession("hubuser", testNow.Add(time.Hour))
	session.Claims.Email = "hubuser@example.com"
	session.Claims.SessionID = "session-1234"
	session.Claims.UUID = "uuid-5678"
	p := newTestProvider(t, &fakeHub{session: session}, nil)
	recorder := recordSpans(p)

	for range 2 {
		_, err := p.GetAuthConfig("docker.io")
		assert.NilError(t, err)
	}

	assertNotRecorded(t, recorder.Ended(), "hubuser", "desktop-oauth-token", "session-1234", "uuid-5678")
}

func exceptionMessage(t *testing.T, s sdktrace.ReadOnlySpan) string {
	t.Helper()
	for _, e := range s.Events() {
		if e.Name != "exception" {
			continue
		}
		for _, kv := range e.Attributes {
			if kv.Key == "exception.message" {
				return kv.Value.AsString()
			}
		}
	}
	t.Fatalf("span %s has no exception event", s.Name())
	return ""
}

func TestDesktopTelemetry_RecordsErrorMessages(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "alice")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	socket := filepath.Join(home, ".cache", "docker-secrets-engine", "engine.sock")
	connErr := &seclient.ConnectError{
		Reason:     seclient.ReasonNotRunning,
		SocketPath: socket,
		Err:        errors.New("dial unix " + socket + ": connect: no such file or directory"),
	}
	p := newTestProvider(t, &fakeHub{sessionErr: connErr}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)

	spans := spansByName(recorder.Ended())
	want := strings.ReplaceAll(connErr.Error(), home, "~")
	for _, name := range []string{"secrets-engine/desktop-session", "secrets-engine/get-default-session"} {
		s := spans[name]
		assert.Assert(t, s != nil, name)
		assert.Equal(t, s.Status().Description, want, name)
		assert.Equal(t, exceptionMessage(t, s), want, name)
	}
	assertNotRecorded(t, recorder.Ended(), home)
}

func TestDesktopTelemetry_TokenIsJWT(t *testing.T) {
	for name, tc := range map[string]struct {
		token string
		want  bool
	}{
		"jwt":    {token: fakeJWT(testNow.Add(time.Hour)), want: true},
		"opaque": {token: "desktop-oauth-token", want: false},
	} {
		t.Run(name, func(t *testing.T) {
			session := hubSession("hubuser", testNow.Add(time.Hour))
			session.AccessToken = tc.token
			p := newTestProvider(t, &fakeHub{session: session}, nil)
			recorder := recordSpans(p)

			_, err := p.GetAuthConfig("docker.io")
			assert.NilError(t, err)

			root := spansByName(recorder.Ended())["secrets-engine/desktop-session"]
			assert.Assert(t, root != nil)
			assertAttr(t, root, attrTokenIsJWT, tc.want)
		})
	}
}

func TestDesktopTelemetry_CacheHit(t *testing.T) {
	p := newTestProvider(t, &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	before := len(recorder.Ended())

	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)

	spans := recorder.Ended()[before:]
	assert.Equal(t, len(spans), 1, "a cache hit must not call the secrets engine")
	hit := spans[0]
	assert.Equal(t, hit.Name(), "secrets-engine/desktop-session")
	assertAttr(t, hit, attrCache, cacheHit)
	assertAttr(t, hit, attrResult, resultSuccess)
	assert.Equal(t, hit.Status().Code, codes.Ok)
}

func TestDesktopAuthProvider_CallerCancellation(t *testing.T) {
	userCancel := errors.New("user pressed Ctrl-C")
	for name, cancelWith := range map[string]error{
		"canceled":          nil,
		"canceled by cause": userCancel,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			hub := &fakeHub{
				session: hubSession("hubuser", testNow.Add(time.Hour)),
				duringSession: func(context.Context) error {
					cancel(cancelWith)
					// The client's error need not wrap context.Canceled.
					return errors.New("transport is closing")
				},
			}
			p := newTestProvider(t, hub, nil)
			recorder := recordSpans(p)
			p.ctx = ctx

			got, err := p.GetAuthConfig("docker.io")
			assert.NilError(t, err)
			assert.DeepEqual(t, got, cliCredentials("docker.io"))

			wantCause := context.Canceled.Error()
			if cancelWith != nil {
				wantCause = cancelWith.Error()
			}
			spans := spansByName(recorder.Ended())
			for _, name := range []string{"secrets-engine/desktop-session", "secrets-engine/get-default-session"} {
				s := spans[name]
				assert.Assert(t, s != nil, name)
				assertAttr(t, s, attrResult, resultCanceled)
				assertAttr(t, s, attrErrorReason, reasonCanceled)
				assertAttr(t, s, attrCancelCause, wantCause)
				assert.Equal(t, s.Status().Code, codes.Unset, name)
				_, hasUnavailable := spanAttr(s, attrSessionUnavailable)
				assert.Assert(t, !hasUnavailable, name)
			}

			// A canceled lookup is no reason to skip Desktop for the next one.
			hub.duringSession = nil
			p.ctx = t.Context()
			got, err = p.GetAuthConfig("docker.io")
			assert.NilError(t, err)
			assert.DeepEqual(t, got, desktopCredentials())
			assert.Equal(t, hub.sessionCalls.Load(), int32(2))
		})
	}
}

func TestDesktopAuthProvider_LookupTimeoutIsAFailure(t *testing.T) {
	hub := &fakeHub{
		duringSession: func(ctx context.Context) error {
			<-ctx.Done()
			return errors.New("rpc: request aborted")
		},
	}
	p := newTestProvider(t, hub, nil)
	p.hub.timeout = time.Millisecond
	recorder := recordSpans(p)

	got, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, cliCredentials("docker.io"))

	spans := spansByName(recorder.Ended())
	for _, name := range []string{"secrets-engine/desktop-session", "secrets-engine/get-default-session"} {
		s := spans[name]
		assert.Assert(t, s != nil, name)
		assertAttr(t, s, attrResult, resultError)
		assertAttr(t, s, attrErrorReason, reasonTimeout)
		assert.Equal(t, s.Status().Code, codes.Error, name)
	}

	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.sessionCalls.Load(), int32(1), "a timed out lookup must back off")
}

func TestDesktopTelemetry_RetryBackoff(t *testing.T) {
	p := newTestProvider(t, &fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	before := len(recorder.Ended())

	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)

	spans := recorder.Ended()[before:]
	assert.Equal(t, len(spans), 1, "a lookup during the retry interval must not call the secrets engine")
	assertAttr(t, spans[0], attrCache, cacheRetryBackoff)
	assertAttr(t, spans[0], attrResult, resultSkipped)
}

func TestDesktopTelemetry_NoSpansForOtherRegistries(t *testing.T) {
	p := newTestProvider(t, &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}, nil)
	recorder := recordSpans(p)

	_, err := p.GetAuthConfig("ghcr.io")
	assert.NilError(t, err)
	assert.Equal(t, len(recorder.Ended()), 0)
}
