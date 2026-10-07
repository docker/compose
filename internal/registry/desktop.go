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
	"encoding/base64"
	"encoding/json"
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
)

const (
	// desktopLookupTimeout bounds a lookup: the SDK has no default timeout and
	// containerd's credential callback has no context.
	desktopLookupTimeout = 5 * time.Second
	sessionExpirySkew    = 30 * time.Second
	// sessionTTL is how long a token without a readable expiry is reused.
	sessionTTL = time.Minute
	// retryInterval is how long Desktop is not asked again after a lookup
	// found no usable session, so a command does not wait on it per image.
	retryInterval = time.Minute
)

var (
	errNoUsername     = errors.New("no username for the session")
	errSessionExpired = errors.New("the session has expired (not signed in to Docker Desktop?)")
)

type hubSessions interface {
	GetDefaultSession(ctx context.Context) (dockerhub.UserSession, error)
	GetDefaultProfile(ctx context.Context) (dockerhub.Profile, error)
}

var _ hubSessions = dockerhub.ClientAuth(nil)

// Sessions belong to the Desktop user, not to a config file, so every provider
// in the process shares them. Each token is only sent to the registry of its
// own environment: the engine resolves docker.io to production even when
// Desktop runs in stage mode.
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
	}
}

// NewDesktopAuthProvider returns an AuthProvider that resolves Docker Hub and
// Docker Hub staging credentials from the Docker Desktop session, and falls
// back to fallback for other registries or when there is no usable session.
//
// The OAuth access token is returned as the account's password, as containerd
// treats an IdentityToken as a refresh token.
func NewDesktopAuthProvider(fallback AuthProvider) AuthProvider {
	return &desktopAuthProvider{
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
	fallback   AuthProvider
	hub        *desktopSession
	stagingHub *desktopSession
}

func (p *desktopAuthProvider) GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error) {
	if session := p.sessionFor(registryHostname); session != nil {
		if auth, ok := session.auth(); ok {
			return auth, nil
		}
	}
	return p.fallback.GetAuthConfig(registryHostname)
}

func (p *desktopAuthProvider) sessionFor(registryHostname string) *desktopSession {
	switch {
	case GetAuthConfigKey(registryHostname) == IndexServer:
		return p.hub
	case isStagingRegistry(registryHostname):
		return p.stagingHub
	default:
		return nil
	}
}

func isStagingRegistry(registryHostname string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(registryHostname, "https://"), "http://")
	return strings.TrimSuffix(host, "/") == StagingRegistryHost
}

// desktopSession caches the Desktop session of one Docker Hub environment.
type desktopSession struct {
	name          string
	serverAddress string
	connect       func() (hubSessions, error)
	timeout       time.Duration
	now           func() time.Time

	// mu serializes lookups so concurrent pulls share one request.
	mu         sync.Mutex
	hub        hubSessions
	retryAt    time.Time
	cached     *clitypes.AuthConfig
	validUntil time.Time
}

func (s *desktopSession) auth() (clitypes.AuthConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now.Before(s.retryAt) {
		return clitypes.AuthConfig{}, false
	}
	if s.cached != nil && now.Before(s.validUntil) {
		return *s.cached, true
	}
	auth, validUntil, err := s.fetch()
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

func (s *desktopSession) fetch() (clitypes.AuthConfig, time.Time, error) {
	if s.hub == nil {
		hub, err := s.connect()
		if err != nil {
			return clitypes.AuthConfig{}, time.Time{}, err
		}
		s.hub = hub
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	session, err := s.hub.GetDefaultSession(ctx)
	if err != nil {
		return clitypes.AuthConfig{}, time.Time{}, err
	}

	// containerd treats a password without a username as a refresh token.
	username := session.Claims.Username
	if username == "" {
		profile, err := s.hub.GetDefaultProfile(ctx)
		if err != nil {
			return clitypes.AuthConfig{}, time.Time{}, fmt.Errorf("resolving the Docker Hub username: %w", err)
		}
		username = profile.Username
	}
	if username == "" {
		return clitypes.AuthConfig{}, time.Time{}, errNoUsername
	}

	now := s.now()
	validUntil := now.Add(sessionTTL)
	if expiresAt, ok := sessionExpiry(session); ok {
		validUntil = expiresAt.Add(-sessionExpirySkew)
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

// sessionExpiry reads the expiry from the session claims, or from the JWT when
// the claims have none.
func sessionExpiry(session dockerhub.UserSession) (time.Time, bool) {
	if exp := session.Claims.ExpiresAt; exp != nil {
		return exp.Time, true
	}
	return jwtExpiry(session.AccessToken)
}

// jwtExpiry does not verify the signature: the registry verifies the token.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		ExpiresAt *dockerhub.NumericDate `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ExpiresAt == nil {
		return time.Time{}, false
	}
	return claims.ExpiresAt.Time, true
}

func logDesktopFallback(name string, err error) {
	if isExpectedMiss(err) {
		logrus.Debugf("no %s session available from Docker Desktop, using Docker CLI credentials: %v", name, err)
		return
	}
	logrus.Warnf("Could not use the %s session from Docker Desktop, using Docker CLI credentials instead: %v", name, err)
}

// isExpectedMiss reports Desktop not running or nobody signed in. An expired
// session is one: Desktop keeps the token fresh while the user is signed in.
// dockerhub.ErrNoDefaultProfile wraps dockerhub.ErrNoSession.
func isExpectedMiss(err error) bool {
	return errors.Is(err, seclient.ErrSecretsEngineNotAvailable) || errors.Is(err, dockerhub.ErrNoSession) || errors.Is(err, errSessionExpired)
}
