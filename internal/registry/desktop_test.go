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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/distribution/reference"
	clitypes "github.com/docker/cli/cli/config/types"
	seclient "github.com/docker/secrets-engine/client"
	"github.com/docker/secrets-engine/client/dockerhub"
	"github.com/docker/secrets-engine/client/realms"
	"github.com/docker/secrets-engine/secrets"
	"github.com/moby/moby/api/pkg/authconfig"
	"gotest.tools/v3/assert"
)

var testNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type fakeHub struct {
	session        dockerhub.UserSession
	sessionErr     error
	profile        dockerhub.Profile
	profileErr     error
	denied         bool
	authorizeErr   error
	authExpiry     time.Time
	pingErr        error
	sessionCalls   atomic.Int32
	authorizeCalls atomic.Int32
	pingCalls      atomic.Int32

	pingBudget      time.Duration
	authorizeBudget time.Duration
}

func budget(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

func (f *fakeHub) Ping(ctx context.Context) error {
	f.pingCalls.Add(1)
	f.pingBudget = budget(ctx)
	return f.pingErr
}

func (f *fakeHub) Authorize(ctx context.Context) (seclient.AuthorizeResponse, error) {
	f.authorizeCalls.Add(1)
	f.authorizeBudget = budget(ctx)
	if f.authorizeErr != nil {
		return seclient.AuthorizeResponse{}, f.authorizeErr
	}
	return seclient.AuthorizeResponse{Allow: !f.denied, Expiry: f.authExpiry}, nil
}

func (f *fakeHub) GetDefaultSession(context.Context) (dockerhub.UserSession, error) {
	f.sessionCalls.Add(1)
	return f.session, f.sessionErr
}

func (f *fakeHub) GetDefaultProfile(context.Context) (dockerhub.Profile, error) {
	return f.profile, f.profileErr
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type recordingFallback struct {
	mu    sync.Mutex
	hosts []string
}

func (f *recordingFallback) GetAuthConfig(registryHostname string) (clitypes.AuthConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hosts = append(f.hosts, registryHostname)
	return cliCredentials(registryHostname), nil
}

func cliCredentials(registryHostname string) clitypes.AuthConfig {
	return clitypes.AuthConfig{Username: "cli-user", Password: "cli-password", ServerAddress: registryHostname}
}

func desktopCredentials() clitypes.AuthConfig {
	return clitypes.AuthConfig{Username: "hubuser", Password: "desktop-oauth-token", ServerAddress: IndexServer}
}

func hubSession(username string, expiresAt time.Time) dockerhub.UserSession {
	s := dockerhub.UserSession{AccessToken: "desktop-oauth-token"}
	s.Claims.Username = username
	if !expiresAt.IsZero() {
		s.Claims.ExpiresAt = &dockerhub.NumericDate{Time: expiresAt}
	}
	return s
}

type testProvider struct {
	*desktopAuthProvider
	fallback *recordingFallback
	clock    *fakeClock
	connects atomic.Int32
	staging  *fakeHub
}

func newTestProvider(hub hubSessions, connectErr error) *testProvider {
	tp := &testProvider{
		fallback: &recordingFallback{},
		clock:    &fakeClock{t: testNow},
		staging:  &fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile},
	}
	tp.desktopAuthProvider = &desktopAuthProvider{
		fallback: tp.fallback,
		hub: &desktopSession{
			name:          "Docker Hub",
			serverAddress: IndexServer,
			connect: func() (hubSessions, error) {
				tp.connects.Add(1)
				if connectErr != nil {
					return nil, connectErr
				}
				return hub, nil
			},
			timeout:          time.Second,
			authorizeTimeout: time.Second,
			now:              tp.clock.now,
		},
		stagingHub: &desktopSession{
			name:          "Docker Hub staging",
			serverAddress: StagingIndexServer,
			connect: func() (hubSessions, error) {
				return tp.staging, nil
			},
			timeout:          time.Second,
			authorizeTimeout: time.Second,
			now:              tp.clock.now,
		},
	}
	return tp
}

func (tp *testProvider) signInToStaging() {
	tp.staging.sessionErr = nil
	tp.staging.session = hubSession("stageuser", testNow.Add(time.Hour))
	tp.staging.session.AccessToken = "staging-oauth-token"
}

func stagingCredentials() clitypes.AuthConfig {
	return clitypes.AuthConfig{Username: "stageuser", Password: "staging-oauth-token", ServerAddress: StagingIndexServer}
}

func TestNewDesktopAuthProvider_SharesOneSessionPerEnvironment(t *testing.T) {
	a, ok := NewDesktopAuthProvider(&recordingFallback{}).(*desktopAuthProvider)
	assert.Assert(t, ok)
	b, ok := NewDesktopAuthProvider(&recordingFallback{}).(*desktopAuthProvider)
	assert.Assert(t, ok)
	assert.Assert(t, a.hub == b.hub, "providers must share the process-wide Docker Hub session")
	assert.Assert(t, a.stagingHub == b.stagingHub, "providers must share the process-wide Docker Hub staging session")
	assert.Assert(t, a.hub != a.stagingHub, "production and staging sessions must be separate")
	assert.Equal(t, a.hub.serverAddress, IndexServer)
	assert.Equal(t, a.stagingHub.serverAddress, StagingIndexServer)
}

func TestDesktopAuthProvider_ProvidersOnOneSessionFetchOnce(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
	p := newTestProvider(hub, nil)
	other := &desktopAuthProvider{fallback: &recordingFallback{}, hub: p.hub, stagingHub: p.stagingHub}

	for _, provider := range []AuthProvider{p, other} {
		got, err := provider.GetAuthConfig("docker.io")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, desktopCredentials())
	}
	assert.Equal(t, hub.sessionCalls.Load(), int32(1))
}

func TestDesktopAuthProvider_DockerHubUsesDesktopSession(t *testing.T) {
	for _, host := range []string{"docker.io", "index.docker.io", "registry-1.docker.io", IndexServer} {
		t.Run(host, func(t *testing.T) {
			hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
			p := newTestProvider(hub, nil)

			got, err := p.GetAuthConfig(host)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, desktopCredentials())
			assert.Equal(t, got.IdentityToken, "", "an IdentityToken is sent as a refresh token")
			assert.Equal(t, len(p.fallback.hosts), 0)
		})
	}
}

func TestDesktopAuthProvider_OtherRegistriesUseFallback(t *testing.T) {
	hosts := []string{
		"ghcr.io", "localhost:5000", "registry.example.com",
		// look-alikes of the staging and DHI registries
		StagingRegistryHost + ".example.com", "example.com/" + StagingRegistryHost,
		DHIRegistryHost + ".example.com", "example.com/" + DHIRegistryHost, "registry." + DHIRegistryHost,
	}
	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
			p := newTestProvider(hub, nil)
			p.signInToStaging()

			got, err := p.GetAuthConfig(host)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, cliCredentials(host))
			assert.Equal(t, p.connects.Load(), int32(0), "Docker Desktop must only be contacted for Docker Hub")
			assert.Equal(t, p.staging.sessionCalls.Load(), int32(0), "Docker Desktop must only be contacted for Docker Hub")
		})
	}
}

func TestDesktopAuthProvider_DHIUsesDockerHubSession(t *testing.T) {
	for _, host := range []string{DHIRegistryHost, DHIIndexServer, DHIIndexServer + "/"} {
		t.Run(host, func(t *testing.T) {
			hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
			p := newTestProvider(hub, nil)
			p.signInToStaging()

			want := desktopCredentials()
			want.ServerAddress = DHIIndexServer
			got, err := p.GetAuthConfig(host)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, want)
			assert.Equal(t, len(p.fallback.hosts), 0)
			assert.Equal(t, p.staging.sessionCalls.Load(), int32(0), "the staging session must not be read for DHI")

			// DHI shares the cached Docker Hub session without changing it.
			got, err = p.GetAuthConfig("docker.io")
			assert.NilError(t, err)
			assert.DeepEqual(t, got, desktopCredentials())
			assert.Equal(t, hub.sessionCalls.Load(), int32(1))
		})
	}
}

func TestDesktopAuthProvider_StagingRegistryUsesStagingSession(t *testing.T) {
	for _, host := range []string{StagingRegistryHost, StagingIndexServer, "https://" + StagingRegistryHost} {
		t.Run(host, func(t *testing.T) {
			hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
			p := newTestProvider(hub, nil)
			p.signInToStaging()

			got, err := p.GetAuthConfig(host)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, stagingCredentials())
			assert.Equal(t, hub.sessionCalls.Load(), int32(0), "the production session must not be read for the staging registry")
		})
	}
}

func TestDesktopAuthProvider_EnvironmentsDoNotMix(t *testing.T) {
	t.Run("staging session is not used for docker.io", func(t *testing.T) {
		// Desktop in stage mode: signed in to staging only.
		p := newTestProvider(&fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile}, nil)
		p.signInToStaging()

		got, err := p.GetAuthConfig("docker.io")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, cliCredentials("docker.io"))

		got, err = p.GetAuthConfig(StagingRegistryHost)
		assert.NilError(t, err)
		assert.DeepEqual(t, got, stagingCredentials())
	})

	t.Run("production session is not used for the staging registry", func(t *testing.T) {
		hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
		p := newTestProvider(hub, nil)

		got, err := p.GetAuthConfig(StagingRegistryHost)
		assert.NilError(t, err)
		assert.DeepEqual(t, got, cliCredentials(StagingRegistryHost))

		got, err = p.GetAuthConfig("docker.io")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, desktopCredentials())
	})
}

func TestDesktopAuthProvider_UsernameFromProfile(t *testing.T) {
	hub := &fakeHub{
		session: hubSession("", testNow.Add(time.Hour)),
		profile: dockerhub.Profile{Username: "hubuser"},
	}
	p := newTestProvider(hub, nil)

	got, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, desktopCredentials())
}

func TestDesktopAuthProvider_FallsBackWithoutUsableSession(t *testing.T) {
	future := testNow.Add(time.Hour)
	tests := []struct {
		name string
		hub  *fakeHub
	}{
		{
			name: "secrets engine not running",
			hub:  &fakeHub{sessionErr: connectError(seclient.ReasonNotRunning)},
		},
		{
			name: "secrets engine socket not accessible",
			hub:  &fakeHub{sessionErr: connectError(seclient.ReasonPermissionDenied)},
		},
		{
			name: "nobody signed in",
			hub:  &fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile},
		},
		{
			name: "access denied",
			hub:  &fakeHub{sessionErr: secrets.ErrAccessDenied},
		},
		{
			name: "no username",
			hub:  &fakeHub{session: hubSession("", future)},
		},
		{
			name: "profile lookup fails",
			hub:  &fakeHub{session: hubSession("", future), profileErr: errors.New("boom")},
		},
		{
			name: "expired",
			hub:  &fakeHub{session: hubSession("hubuser", testNow.Add(-time.Minute))},
		},
		{
			name: "expires within the skew",
			hub:  &fakeHub{session: hubSession("hubuser", testNow.Add(sessionExpirySkew/2))},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestProvider(tc.hub, nil)

			for range 2 {
				got, err := p.GetAuthConfig("docker.io")
				assert.NilError(t, err)
				assert.DeepEqual(t, got, cliCredentials("docker.io"))
			}
			assert.Equal(t, tc.hub.sessionCalls.Load(), int32(1), "Desktop must not be asked again within retryInterval")
		})
	}
}

func TestDesktopAuthProvider_FallsBackWhenClientCannotBeCreated(t *testing.T) {
	p := newTestProvider(nil, errors.New("no socket path"))

	for range 2 {
		got, err := p.GetAuthConfig("docker.io")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, cliCredentials("docker.io"))
	}
	assert.Equal(t, p.connects.Load(), int32(1))
}

func TestDesktopAuthProvider_RetriesAfterInterval(t *testing.T) {
	hub := &fakeHub{sessionErr: dockerhub.ErrNoDefaultProfile}
	p := newTestProvider(hub, nil)

	got, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, cliCredentials("docker.io"))

	hub.sessionErr = nil
	hub.session = hubSession("hubuser", testNow.Add(time.Hour))

	p.clock.advance(retryInterval - time.Second)
	got, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, cliCredentials("docker.io"))
	assert.Equal(t, hub.sessionCalls.Load(), int32(1))

	p.clock.advance(time.Second)
	got, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, desktopCredentials())
	assert.Equal(t, hub.sessionCalls.Load(), int32(2))
}

func TestIsSessionUnavailable(t *testing.T) {
	for _, err := range []error{
		connectError(seclient.ReasonNotRunning),
		fmt.Errorf("get session: %w", connectError(seclient.ReasonNotRunning)),
		connectError(seclient.ReasonPermissionDenied),
		connectError(seclient.ReasonTimeout),
		connectError(seclient.ReasonUnknown),
		dockerhub.ErrNoSession,
		dockerhub.ErrNoDefaultProfile,
		errSessionExpired,
	} {
		assert.Assert(t, isSessionUnavailable(err), err)
	}
	for _, err := range []error{
		secrets.ErrAccessDenied,
		errNoUsername,
		context.DeadlineExceeded,
	} {
		assert.Assert(t, !isSessionUnavailable(err), err)
	}
}

func connectError(reason seclient.ConnectReason) error {
	return &seclient.ConnectError{
		Reason:     reason,
		SocketPath: "/run/engine.sock",
		Err:        errors.New("dial unix /run/engine.sock"),
	}
}

type recordingEngine struct {
	patterns    []secrets.Pattern
	versionErr  error
	versionCall int
}

func (a *recordingEngine) Authorize(_ context.Context, patterns ...secrets.Pattern) (seclient.AuthorizeResponse, error) {
	a.patterns = append(a.patterns, patterns...)
	return seclient.AuthorizeResponse{Allow: true}, nil
}

func (a *recordingEngine) Version(context.Context) (seclient.DaemonVersion, error) {
	a.versionCall++
	return seclient.DaemonVersion{}, a.versionErr
}

func TestDesktopHub_PingAsksForTheVersion(t *testing.T) {
	e := &recordingEngine{}
	assert.NilError(t, desktopHub{engine: e}.Ping(t.Context()))
	assert.Equal(t, e.versionCall, 1)

	e.versionErr = connectError(seclient.ReasonNotRunning)
	assert.ErrorIs(t, desktopHub{engine: e}.Ping(t.Context()), seclient.ErrSecretsEngineNotRunning)
}

func TestDesktopHub_PingReportsATimeoutAsTheEngineNotResponding(t *testing.T) {
	e := &recordingEngine{versionErr: fmt.Errorf("deadline_exceeded: %w", context.DeadlineExceeded)}
	err := desktopHub{engine: e, socketPath: "/run/engine.sock"}.Ping(t.Context())

	assert.ErrorIs(t, err, seclient.ErrSecretsEngineTimeout)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "secrets engine did not respond in time at /run/engine.sock")
	assert.Assert(t, isSessionUnavailable(err), err)
}

func TestDesktopHub_PingKeepsOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	e := &recordingEngine{versionErr: boom}
	assert.Equal(t, desktopHub{engine: e}.Ping(t.Context()), boom)
}

func TestDesktopHub_AuthorizesDockerAuthRealm(t *testing.T) {
	a := &recordingEngine{}
	resp, err := desktopHub{engine: a}.Authorize(t.Context())
	assert.NilError(t, err)
	assert.Assert(t, resp.Allow)
	assert.Equal(t, len(a.patterns), 1)
	assert.Equal(t, a.patterns[0].String(), "docker/auth/**")
	for _, realm := range []secrets.Pattern{
		realms.DockerHubAuthentication,
		realms.DockerHubAuthenticationMetadata,
		realms.DockerHubStagingAuthentication,
		realms.DockerHubStagingAuthenticationMetadata,
	} {
		assert.Assert(t, a.patterns[0].Contains(realm), "docker/auth/** must cover %s", realm)
	}
}

func TestDesktopAuthProvider_AuthorizesOnceBeforeReadingSessions(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", time.Time{})}
	p := newTestProvider(hub, nil)

	for range 3 {
		got, err := p.GetAuthConfig("docker.io")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, desktopCredentials())
		p.clock.advance(sessionTTL)
	}
	assert.Equal(t, hub.sessionCalls.Load(), int32(3))
	assert.Equal(t, hub.authorizeCalls.Load(), int32(1))
	assert.Equal(t, hub.pingCalls.Load(), int32(1))
}

func TestDesktopAuthProvider_WaitsForThePromptOnlyAfterTheEngineAnswers(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
	p := newTestProvider(hub, nil)
	p.hub.timeout = 5 * time.Second
	p.hub.authorizeTimeout = time.Minute

	got, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.DeepEqual(t, got, desktopCredentials())
	assert.Assert(t, hub.pingBudget > 0 && hub.pingBudget <= 5*time.Second, "ping budget %s", hub.pingBudget)
	assert.Assert(t, hub.authorizeBudget > 5*time.Second, "authorize budget %s", hub.authorizeBudget)
}

func TestDesktopAuthProvider_FallsBackWhenTheEngineDoesNotAnswer(t *testing.T) {
	for name, pingErr := range map[string]error{
		"not running":     connectError(seclient.ReasonNotRunning),
		"does not answer": connectError(seclient.ReasonTimeout),
	} {
		t.Run(name, func(t *testing.T) {
			hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour)), pingErr: pingErr}
			p := newTestProvider(hub, nil)

			for range 2 {
				got, err := p.GetAuthConfig("docker.io")
				assert.NilError(t, err)
				assert.DeepEqual(t, got, cliCredentials("docker.io"))
			}
			assert.Equal(t, hub.pingCalls.Load(), int32(1), "Desktop must not be asked again within retryInterval")
			assert.Equal(t, hub.authorizeCalls.Load(), int32(0), "authorization must not wait on an engine that does not answer")
			assert.Equal(t, hub.sessionCalls.Load(), int32(0))
		})
	}
}

func TestDesktopAuthProvider_AuthorizesAgainAfterDecisionExpires(t *testing.T) {
	hub := &fakeHub{
		session:    hubSession("hubuser", time.Time{}),
		authExpiry: testNow.Add(sessionTTL + time.Second),
	}
	p := newTestProvider(hub, nil)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	p.clock.advance(sessionTTL)
	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.authorizeCalls.Load(), int32(1))

	p.clock.advance(sessionTTL)
	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.authorizeCalls.Load(), int32(2))
	assert.Equal(t, hub.sessionCalls.Load(), int32(3))
}

func TestDesktopAuthProvider_FallsBackWhenNotAuthorized(t *testing.T) {
	tests := []struct {
		name string
		hub  *fakeHub
	}{
		{
			name: "denied",
			hub:  &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour)), denied: true},
		},
		{
			name: "access denied error",
			hub:  &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour)), authorizeErr: secrets.ErrAccessDenied},
		},
		{
			name: "prompt not answered",
			hub:  &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour)), authorizeErr: context.DeadlineExceeded},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestProvider(tc.hub, nil)

			for range 2 {
				got, err := p.GetAuthConfig("docker.io")
				assert.NilError(t, err)
				assert.DeepEqual(t, got, cliCredentials("docker.io"))
			}
			assert.Equal(t, tc.hub.pingCalls.Load(), int32(1), "Desktop must not be asked again within retryInterval")
			assert.Equal(t, tc.hub.authorizeCalls.Load(), int32(1), "Desktop must not be asked again within retryInterval")
			assert.Equal(t, tc.hub.sessionCalls.Load(), int32(0), "the session must not be read without authorization")
		})
	}
}

func TestDesktopAuthProvider_ReusesSessionUntilExpiry(t *testing.T) {
	expiresAt := testNow.Add(10 * time.Minute)
	hub := &fakeHub{session: hubSession("hubuser", expiresAt)}
	p := newTestProvider(hub, nil)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	p.clock.advance(5 * time.Minute)
	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.sessionCalls.Load(), int32(1))

	// Desktop has refreshed the token by the time the cached one is about to
	// expire.
	hub.session = hubSession("hubuser", expiresAt.Add(time.Hour))
	hub.session.AccessToken = "refreshed-token"
	p.clock.advance(5*time.Minute - sessionExpirySkew)

	got, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, got.Password, "refreshed-token")
	assert.Equal(t, hub.sessionCalls.Load(), int32(2))
}

func TestDesktopAuthProvider_RefetchesSessionWithoutExpiry(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", time.Time{})}
	p := newTestProvider(hub, nil)

	_, err := p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	p.clock.advance(sessionTTL - time.Second)
	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.sessionCalls.Load(), int32(1))

	p.clock.advance(time.Second)
	_, err = p.GetAuthConfig("docker.io")
	assert.NilError(t, err)
	assert.Equal(t, hub.sessionCalls.Load(), int32(2))
}

func TestDesktopAuthProvider_ConcurrentLookupsShareOneFetch(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
	p := newTestProvider(hub, nil)

	const lookups = 20
	results := make([]clitypes.AuthConfig, lookups)
	var wg sync.WaitGroup
	for i := range lookups {
		wg.Go(func() {
			auth, err := p.GetAuthConfig("docker.io")
			assert.Check(t, err)
			results[i] = auth
		})
	}
	wg.Wait()

	for _, auth := range results {
		assert.DeepEqual(t, auth, desktopCredentials())
	}
	assert.Equal(t, hub.sessionCalls.Load(), int32(1))
	assert.Equal(t, p.connects.Load(), int32(1))
}

func TestEncodedAuth_DesktopSession(t *testing.T) {
	hub := &fakeHub{session: hubSession("hubuser", testNow.Add(time.Hour))}
	p := newTestProvider(hub, nil)
	ref, err := reference.ParseNormalizedNamed("alpine")
	assert.NilError(t, err)

	encoded, err := EncodedAuth(ref, p)
	assert.NilError(t, err)

	decoded, err := authconfig.Decode(encoded)
	assert.NilError(t, err)
	assert.Equal(t, decoded.Username, "hubuser")
	assert.Equal(t, decoded.Password, "desktop-oauth-token")
	assert.Equal(t, decoded.ServerAddress, IndexServer)
	assert.Equal(t, decoded.IdentityToken, "")
}
