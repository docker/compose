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
	// desktopLookupTimeout bounds a Docker Hub session lookup against the
	// Docker Desktop secrets engine. The engine may hold a request open on
	// user interaction and the SDK applies no timeout by default, while some
	// callers (containerd's credential callback) have no context to cancel
	// with, so the lookup carries its own deadline.
	desktopLookupTimeout = 5 * time.Second
	// sessionExpirySkew treats a session as expired slightly early, so a
	// token is not handed out just before it lapses mid-request.
	sessionExpirySkew = 30 * time.Second
	// sessionTTL bounds how long a session without an expiry claim is reused
	// before it is fetched again.
	sessionTTL = time.Minute
)

var (
	errNoUsername     = errors.New("no username for the session")
	errSessionExpired = errors.New("the session has expired (not signed in to Docker Desktop?)")
)

// hubSessions is the subset of the secrets engine Docker Hub accessor
// (dockerhub.ClientAuth) used to read the signed-in account.
type hubSessions interface {
	GetDefaultSession(ctx context.Context) (dockerhub.UserSession, error)
	GetDefaultProfile(ctx context.Context) (dockerhub.Profile, error)
}

var _ hubSessions = dockerhub.ClientAuth(nil)

// The shared sessions back every provider returned by NewDesktopAuthProvider.
// A session belongs to the user signed in to Docker Desktop rather than to a
// Docker CLI config file, so the compose service, the oci:// loader and the
// dry-run client share them and Docker Desktop is asked once per process
// rather than once by each of them.
//
// Docker Desktop keeps production and staging sessions in separate secrets
// engine realms. Each is only used for the registry of its own environment:
// the engine resolves docker.io to the production registry even when Desktop
// runs in stage mode, so a staging token must never be sent there, nor a
// production token to the staging registry.
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

// NewDesktopAuthProvider returns an AuthProvider that resolves Docker Hub
// credentials from the Docker Desktop secrets engine, and falls back to
// fallback (typically the Docker CLI config file and its credential helpers)
// for every other registry, or when Desktop is not running, nobody is signed
// in, or the session cannot be used. The Docker Hub staging registry
// (StagingRegistryHost) is resolved the same way from the staging session.
//
// The Desktop OAuth access token is returned as the password of the signed-in
// account, so it travels like any username/password credential: containerd
// exchanges it with an OAuth password grant, and the engine receives it in
// X-Registry-Auth. It is deliberately not returned as an IdentityToken, which
// containerd treats as a refresh token.
//
// The engine is contacted lazily, on the first Docker Hub lookup.
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

// desktopAuthProvider resolves Docker Hub credentials from the Docker Desktop
// session of the matching environment, and every other registry, or Docker
// Hub whenever Desktop has no usable session, from the fallback provider.
type desktopAuthProvider struct {
	fallback   AuthProvider
	hub        *desktopSession
	stagingHub *desktopSession
}

// GetAuthConfig implements AuthProvider.
func (p *desktopAuthProvider) GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error) {
	if session := p.sessionFor(registryHostname); session != nil {
		if auth, ok := session.auth(); ok {
			return auth, nil
		}
	}
	return p.fallback.GetAuthConfig(registryHostname)
}

// sessionFor returns the Desktop session for the environment registryHostname
// belongs to, or nil when it is not a Docker Hub registry.
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

// isStagingRegistry reports whether registryHostname, a host or an index
// server URL, is the Docker Hub staging registry.
func isStagingRegistry(registryHostname string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(registryHostname, "https://"), "http://")
	return strings.TrimSuffix(host, "/") == StagingRegistryHost
}

// desktopSession reads and caches the session of the account signed in to
// Docker Desktop, for one Docker Hub environment.
type desktopSession struct {
	// name identifies the environment in logs.
	name string
	// serverAddress is the credentials key of the environment.
	serverAddress string
	connect       func() (hubSessions, error)
	timeout       time.Duration
	now           func() time.Time

	// mu serializes lookups, so concurrent pulls share a single round-trip
	// to the engine.
	mu  sync.Mutex
	hub hubSessions
	// unavailable records that Desktop could not provide a usable session.
	// Later lookups go straight to the fallback instead of asking (and
	// possibly waiting out the timeout) again for every image.
	unavailable bool
	cached      *clitypes.AuthConfig
	validUntil  time.Time
}

// auth returns the Docker Hub credentials of the Desktop session, and whether
// there is a usable one.
func (s *desktopSession) auth() (clitypes.AuthConfig, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.unavailable {
		return clitypes.AuthConfig{}, false
	}
	if s.cached != nil && s.now().Before(s.validUntil) {
		return *s.cached, true
	}
	auth, validUntil, err := s.fetch()
	if err != nil {
		s.unavailable = true
		s.cached = nil
		logDesktopFallback(s.name, err)
		return clitypes.AuthConfig{}, false
	}
	logrus.Debugf("using the %s session of %q from Docker Desktop", s.name, auth.Username)
	s.cached, s.validUntil = &auth, validUntil
	return auth, true
}

// fetch reads the default Desktop session and converts it to credentials,
// along with the time until which they may be reused.
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

	// containerd reads an empty username as "the secret is a refresh token"
	// and switches to a refresh_token grant, so the token is only usable
	// together with the account name.
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

// sessionExpiry returns when the session's access token expires: from the
// session claims, or, when the payload carries none, from the token itself.
// An expiry that cannot be read is tolerated, the token is then reused for
// sessionTTL only.
func sessionExpiry(session dockerhub.UserSession) (time.Time, bool) {
	if exp := session.Claims.ExpiresAt; exp != nil {
		return exp.Time, true
	}
	return jwtExpiry(session.AccessToken)
}

// jwtExpiry reads the exp claim of a JWT without verifying its signature: it
// only decides whether the token is still worth sending, the registry
// verifies it.
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

// logDesktopFallback reports why Docker Hub credentials come from the
// fallback. Desktop not running and nobody signed in are routine and only
// logged at debug level. That includes an expired session: Desktop keeps the
// stored access token fresh while the user is signed in, so an expired one is
// left over from an earlier sign-in. Anything else is surprising enough to
// warn about.
func logDesktopFallback(name string, err error) {
	if errors.Is(err, seclient.ErrSecretsEngineNotAvailable) || errors.Is(err, dockerhub.ErrNoSession) || errors.Is(err, errSessionExpired) {
		logrus.Debugf("no %s session available from Docker Desktop, using Docker CLI credentials: %v", name, err)
		return
	}
	logrus.Warnf("Could not use the %s session from Docker Desktop, using Docker CLI credentials instead: %v", name, err)
}
