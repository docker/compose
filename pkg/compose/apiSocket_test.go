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

package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	clitypes "github.com/docker/cli/cli/config/types"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/mocks"
)

// linuxContextInfo is the context of a Linux engine.
type linuxContextInfo struct{}

func (linuxContextInfo) CurrentContext() string         { return "default" }
func (linuxContextInfo) ServerOSType() string           { return "linux" }
func (linuxContextInfo) BuildKitEnabled() (bool, error) { return false, nil }

// hostRegistryConfig is the CLI configuration of a host that is logged in to a
// registry: what use_api_socket has to hand over.
func hostRegistryConfig() *configfile.ConfigFile {
	return &configfile.ConfigFile{AuthConfigs: map[string]clitypes.AuthConfig{
		"registry.example.invalid": {Username: "user", Password: "secret"},
	}}
}

// newAPISocketTestService is a service on a Linux engine whose CLI has the
// given configuration.
func newAPISocketTestService(t *testing.T, config *configfile.ConfigFile) *composeService {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(config).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("unix:///var/run/docker.sock").AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(progress.Noop{}), WithContextInfo(linuxContextInfo{}))
	assert.NilError(t, err)
	return svc.(*composeService)
}

func apiSocketService(name string, env types.MappingWithEquals) types.ServiceConfig {
	return types.ServiceConfig{Name: name, ContainerSpec: types.ContainerSpec{UseAPISocket: true, Environment: env}}
}

func apiSocketProject(services ...types.ServiceConfig) *types.Project {
	project := &types.Project{Name: "test", Services: types.Services{}, Configs: types.Configs{}}
	for _, service := range services {
		project.Services[service.Name] = service
	}
	return project
}

func dockerConfigEnv(value string) types.MappingWithEquals {
	return types.MappingWithEquals{"DOCKER_CONFIG": &value}
}

// TestUseAPISocketIsIdempotent verifies that a project which already went
// through useAPISocket -- the plan's own project, handed to a later plan such
// as a watch rebuild -- is not transformed a second time: the socket and the
// config file would be mounted twice.
func TestUseAPISocketIsIdempotent(t *testing.T) {
	svc := newAPISocketTestService(t, hostRegistryConfig())
	project := apiSocketProject(apiSocketService("app", types.MappingWithEquals{}))

	once, err := svc.useAPISocket(project)
	assert.NilError(t, err)
	service := once.Services["app"]
	assert.Equal(t, len(service.Volumes), 1, "volumes: %v", service.Volumes)
	assert.Equal(t, len(service.Configs), 1, "configs: %v", service.Configs)

	twice, err := svc.useAPISocket(once)
	assert.NilError(t, err)
	service = twice.Services["app"]
	assert.Equal(t, len(service.Volumes), 1, "volumes: %v", service.Volumes)
	assert.Equal(t, len(service.Configs), 1, "configs: %v", service.Configs)
}

// TestUseAPISocketCredentialsAreForRootOnly pins who can read the registry
// credentials handed to the container: like the file of `docker run
// --use-api-socket`, it is readable by its owner only, and it stays root's --
// no UID/GID is set, so that the engine gives the file the default ownership
// -- rather than the 0444 any config file gets by default, which another user
// of the container could read, whether or not it can use the socket.
func TestUseAPISocketCredentialsAreForRootOnly(t *testing.T) {
	svc := newAPISocketTestService(t, hostRegistryConfig())

	project, err := svc.useAPISocket(apiSocketProject(apiSocketService("app", types.MappingWithEquals{})))

	assert.NilError(t, err)
	service := project.Services["app"]
	assert.Equal(t, len(service.Configs), 1, "configs: %v", service.Configs)
	config := service.Configs[0]
	assert.Equal(t, config.Source, "#apisocket")
	assert.Equal(t, config.Target, "/run/secrets/docker/config.json")
	assert.Assert(t, config.Mode != nil, "the mode must be explicit, not the 0444 default")
	assert.Equal(t, *config.Mode, types.FileMode(0o600))
	assert.Equal(t, config.UID, "")
	assert.Equal(t, config.GID, "")
	assert.Equal(t, *service.Environment["DOCKER_CONFIG"], "/run/secrets/docker")
	assert.Assert(t, project.Configs["#apisocket"].Content != "", "the config carries the credentials")
}

// TestUseAPISocketHonorsDockerConfig pins that a service which sets
// DOCKER_CONFIG itself gets the socket but no credentials, as with `docker run
// --use-api-socket`: it is taken to know where its credentials are, and its
// DOCKER_CONFIG is left as it set it.
func TestUseAPISocketHonorsDockerConfig(t *testing.T) {
	svc := newAPISocketTestService(t, hostRegistryConfig())

	project, err := svc.useAPISocket(apiSocketProject(
		apiSocketService("explicit", dockerConfigEnv("/elsewhere")),
		apiSocketService("default", types.MappingWithEquals{}),
	))

	assert.NilError(t, err)
	explicit := project.Services["explicit"]
	assert.Equal(t, len(explicit.Volumes), 1, "the socket is still mounted: %v", explicit.Volumes)
	assert.Equal(t, len(explicit.Configs), 0, "no credentials for a service setting DOCKER_CONFIG: %v", explicit.Configs)
	assert.Equal(t, *explicit.Environment["DOCKER_CONFIG"], "/elsewhere")
	// the others still get them
	other := project.Services["default"]
	assert.Equal(t, len(other.Configs), 1, "configs: %v", other.Configs)
	assert.Equal(t, *other.Environment["DOCKER_CONFIG"], "/run/secrets/docker")
}

// TestUseAPISocketWithoutCredentials pins that a host with no registry
// credentials hands over nothing -- no config file, no DOCKER_CONFIG pointing
// at a file that does not exist -- while the socket is still mounted.
func TestUseAPISocketWithoutCredentials(t *testing.T) {
	svc := newAPISocketTestService(t, &configfile.ConfigFile{})

	project, err := svc.useAPISocket(apiSocketProject(apiSocketService("app", nil)))

	assert.NilError(t, err)
	service := project.Services["app"]
	assert.Equal(t, len(service.Volumes), 1, "the socket is still mounted: %v", service.Volumes)
	assert.Equal(t, len(service.Configs), 0, "configs: %v", service.Configs)
	_, set := service.Environment["DOCKER_CONFIG"]
	assert.Assert(t, !set, "DOCKER_CONFIG must not point at a file that is not there")

	// and it is not transformed again
	again, err := svc.useAPISocket(project)
	assert.NilError(t, err)
	assert.Equal(t, len(again.Services["app"].Volumes), 1, "volumes: %v", again.Services["app"].Volumes)
}

// TestUseAPISocketLooksCredentialsUpOnlyForWhoGetsThem pins that the
// credentials are not looked up -- the credential helper of the host is not
// asked -- when every service sets DOCKER_CONFIG, and that a failing lookup is
// reported when one service is to get them.
func TestUseAPISocketLooksCredentialsUpOnlyForWhoGetsThem(t *testing.T) {
	failing := &configfile.ConfigFile{CredentialsStore: "no-such-credential-helper"}

	svc := newAPISocketTestService(t, failing)
	_, err := svc.useAPISocket(apiSocketProject(apiSocketService("explicit", dockerConfigEnv("/elsewhere"))))
	assert.NilError(t, err)

	svc = newAPISocketTestService(t, failing)
	_, err = svc.useAPISocket(apiSocketProject(apiSocketService("default", types.MappingWithEquals{})))
	assert.ErrorContains(t, err, "resolving credentials failed")
}
