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
	"strings"
	"sync"
	"time"

	clitypes "github.com/docker/cli/cli/config/types"
	seclient "github.com/docker/secrets-engine/client"
	"github.com/docker/secrets-engine/client/dockerhub"
	"github.com/docker/secrets-engine/x/api"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

const (
	// desktopLookupTimeout limits connection time in case of hanging or
	// unexpected request delay by the secrets engine.
	desktopLookupTimeout = 5 * time.Second
	sessionExpirySkew    = 30 * time.Second
	// sessionTTL is how long a session without an expiry claim is reused.
	sessionTTL = time.Minute
	// retryInterval is how long Desktop is not asked again after a failed lookup.
	retryInterval = time.Minute
)

var (
	errNoUsername     = errors.New("no username for the session")
	errSessionExpired = errors.New("the session has expired (not signed in to Docker Desktop?)")
	errLookupTimeout  = errors.New("the secrets engine did not answer in time")
)

type hubSessions interface {
	GetDefaultSession(ctx context.Context) (dockerhub.UserSession, error)
	GetDefaultProfile(ctx context.Context) (dockerhub.Profile, error)
}

var _ hubSessions = dockerhub.ClientAuth(nil)

var (
	sharedHubSession        = newDesktopSession("Docker Hub", IndexServer)
	sharedStagingHubSession = newDesktopSession("Docker Hub staging", StagingIndexServer, dockerhub.Staging())
)

func newDesktopSession(name, serverAddress string, opts ...dockerhub.Option) *desktopSession {
	return &desktopSession{
		name:          name,
		serverAddress: serverAddress,
		connect: func() (hubSessions, error) {
			return connectDesktop(opts...)
		},
		timeout: desktopLookupTimeout,
		now:     time.Now,
		tracer:  otel.Tracer(""),
	}
}

// NewDesktopAuthProvider returns an AuthProvider that uses the Docker Desktop
// session for Docker Hub, and fallback otherwise.
func NewDesktopAuthProvider(ctx context.Context, fallback AuthProvider) AuthProvider {
	return &desktopAuthProvider{
		ctx:        ctx,
		fallback:   fallback,
		hub:        sharedHubSession,
		stagingHub: sharedStagingHubSession,
	}
}

func connectDesktop(opts ...dockerhub.Option) (hubSessions, error) {
	c, err := seclient.New(
		seclient.WithSocketPath(api.DesktopSocketPath()),
		seclient.WithTimeout(desktopLookupTimeout),
	)
	if err != nil {
		return nil, err
	}
	return c.HubAuth(opts...), nil
}

type desktopAuthProvider struct {
	// ctx is the operation the provider was created for. GetAuthConfig takes
	// no context, so lookups use this one.
	ctx        context.Context
	fallback   AuthProvider
	hub        *desktopSession
	stagingHub *desktopSession
}

func (p *desktopAuthProvider) GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error) {
	if session := p.sessionFor(registryHostname); session != nil {
		if auth, ok := session.auth(p.ctx); ok {
			if registryHost(registryHostname) == DHIRegistryHost {
				auth.ServerAddress = DHIIndexServer
			}
			return auth, nil
		}
	}
	return p.fallback.GetAuthConfig(registryHostname)
}

func (p *desktopAuthProvider) sessionFor(registryHostname string) *desktopSession {
	switch {
	case GetAuthConfigKey(registryHostname) == IndexServer, registryHost(registryHostname) == DHIRegistryHost:
		return p.hub
	case registryHost(registryHostname) == StagingRegistryHost:
		return p.stagingHub
	default:
		return nil
	}
}

// registryHost strips the scheme and trailing slash from a registry address.
func registryHost(registryHostname string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(registryHostname, "https://"), "http://")
	return strings.TrimSuffix(host, "/")
}

// desktopSession caches the Desktop session of one Docker Hub environment.
type desktopSession struct {
	name          string
	serverAddress string
	connect       func() (hubSessions, error)
	timeout       time.Duration
	now           func() time.Time
	tracer        trace.Tracer

	// mu serializes lookups so concurrent pulls share one request.
	mu         sync.Mutex
	hub        hubSessions
	retryAt    time.Time
	cached     *clitypes.AuthConfig
	validUntil time.Time
}

func (s *desktopSession) auth(ctx context.Context) (clitypes.AuthConfig, bool) {
	ctx, span := s.tracer.Start(ctx, spanPrefix+"desktop-session",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrEnvironment.String(s.name)),
	)
	defer span.End()

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now.Before(s.retryAt) {
		span.SetAttributes(attrCache.String(cacheRetryBackoff), attrResult.String(resultSkipped))
		return clitypes.AuthConfig{}, false
	}
	if s.cached != nil && now.Before(s.validUntil) {
		span.SetAttributes(attrCache.String(cacheHit))
		recordResult(ctx, span, nil)
		return *s.cached, true
	}
	span.SetAttributes(attrCache.String(cacheMiss))
	auth, validUntil, err := s.fetch(ctx)
	recordResult(ctx, span, err)
	if err != nil && ctx.Err() != nil {
		logrus.Debugf("looking up the %s session from Docker Desktop was canceled: %v", s.name, context.Cause(ctx))
		return clitypes.AuthConfig{}, false
	}
	if err != nil {
		s.retryAt = s.now().Add(retryInterval)
		s.cached = nil
		logDesktopFallback(s.name, err)
		return clitypes.AuthConfig{}, false
	}
	logrus.Debugf("using the %s session of %q from Docker Desktop", s.name, auth.Username)
	s.cached, s.validUntil = &auth, validUntil
	return auth, true
}

func (s *desktopSession) fetch(ctx context.Context) (clitypes.AuthConfig, time.Time, error) {
	span := trace.SpanFromContext(ctx)

	if s.hub == nil {
		hub, err := traceCall(ctx, s.tracer, "connect", func(context.Context) (hubSessions, error) {
			return s.connect()
		})
		if err != nil {
			return clitypes.AuthConfig{}, time.Time{}, err
		}
		s.hub = hub
	}

	ctx, cancel := context.WithTimeoutCause(ctx, s.timeout, errLookupTimeout)
	defer cancel()

	session, err := traceCall(ctx, s.tracer, "get-default-session", s.hub.GetDefaultSession)
	if err != nil {
		return clitypes.AuthConfig{}, time.Time{}, withLookupTimeout(ctx, err)
	}

	// containerd treats a password without a username as a refresh token.
	username := session.Claims.Username
	span.SetAttributes(attrUsernameFromProfile.Bool(username == ""))
	if username == "" {
		profile, err := traceCall(ctx, s.tracer, "get-default-profile", s.hub.GetDefaultProfile)
		if err != nil {
			return clitypes.AuthConfig{}, time.Time{}, fmt.Errorf("resolving the Docker Hub username: %w", withLookupTimeout(ctx, err))
		}
		username = profile.Username
	}
	if username == "" {
		return clitypes.AuthConfig{}, time.Time{}, errNoUsername
	}

	now := s.now()
	validUntil := now.Add(sessionTTL)
	if exp := session.Claims.ExpiresAt; exp != nil {
		validUntil = exp.Add(-sessionExpirySkew)
		if !now.Before(validUntil) {
			return clitypes.AuthConfig{}, time.Time{}, errSessionExpired
		}
	}

	return clitypes.AuthConfig{
		Username:      username,
		Password:      session.AccessToken,
		ServerAddress: s.serverAddress,
	}, validUntil, nil
}

// withLookupTimeout marks err as caused by the lookup timeout when that is why
// ctx ended, since client errors need not say so.
func withLookupTimeout(ctx context.Context, err error) error {
	if errors.Is(context.Cause(ctx), errLookupTimeout) && !errors.Is(err, errLookupTimeout) {
		return fmt.Errorf("%w: %w", errLookupTimeout, err)
	}
	return err
}

func logDesktopFallback(name string, err error) {
	if isSessionUnavailable(err) {
		logrus.Debugf("no %s session available from Docker Desktop, using Docker CLI credentials: %v", name, err)
		return
	}
	logrus.Warnf("could not use the %s session from Docker Desktop, using Docker CLI credentials instead: %v", name, err)
}

// isSessionUnavailable reports whether err means there is no Desktop session
// to use, including any failure to reach the secrets engine.
func isSessionUnavailable(err error) bool {
	if _, ok := errors.AsType[*seclient.ConnectError](err); ok {
		return true
	}
	return errors.Is(err, dockerhub.ErrNoSession) || errors.Is(err, errSessionExpired)
}
