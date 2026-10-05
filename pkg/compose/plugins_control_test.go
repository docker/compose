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

package compose

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/desktop"
	"github.com/docker/compose/v5/pkg/mocks"
)

// TestExecutePlugin_GetServiceConfig runs executePlugin against a fake
// provider (this test binary re-executed, see TestHelperProviderConfig): each
// get-service-config message must be answered on the provider's stdin with
// one JSON line holding the in-memory service's canonical configuration.
func TestExecutePlugin_GetServiceConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(mocks.NewMockAPIClient(mockCtrl)).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderConfig")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name: "db",
		Provider: &types.ServiceProviderConfig{
			Type:    "sbx",
			Options: types.MultiOptions{"template": {"agent:latest"}},
		},
	}
	variables, err := svc.(*composeService).executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "up", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["TEMPLATE"], "agent:latest")
	// the channel stays usable for more than one request
	assert.Equal(t, variables.prefixed["TEMPLATE_AGAIN"], "agent:latest")
}

// TestHelperProviderConfig is not a test: it is the fake provider process
// spawned by TestExecutePlugin_GetServiceConfig.
func TestHelperProviderConfig(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetServiceConfig")
	}
	responses := json.NewDecoder(os.Stdin)
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	getServiceConfig := func() (template string, ok bool) {
		emit(JsonMessage{Type: GetServiceConfigType})
		var config struct {
			Provider struct {
				Options map[string][]string `json:"options"`
			} `json:"provider"`
		}
		if err := responses.Decode(&config); err != nil {
			emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("reading service config: %v", err)})
			return "", false
		}
		return config.Provider.Options["template"][0], true
	}

	template, ok := getServiceConfig()
	if !ok {
		os.Exit(0)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "TEMPLATE=" + template})
	if template, ok = getServiceConfig(); ok {
		emit(JsonMessage{Type: SetEnvType, Message: "TEMPLATE_AGAIN=" + template})
	}
	os.Exit(0)
}

// TestExecutePlugin_GetServiceConfigResolvesBuildOnlyImage is a regression
// test: a service declaring only build: (no image:) must still answer
// get-service-config with a non-empty image — the name the image phase
// built and tagged (api.GetImageNameOrDefault), not the YAML-declared
// service.Image, which compose-go never fills in for this case. A provider
// consuming a build-only service (e.g. a sandbox provider building its own
// runtime image) otherwise sees an empty "image" field and rejects the
// service outright. The build directive itself must NOT be forwarded: a
// provider has no builder to run it against, only an image identity to run.
func TestExecutePlugin_GetServiceConfigResolvesBuildOnlyImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(mocks.NewMockAPIClient(mockCtrl)).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderConfigImage")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:         "api",
		WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
		Provider: &types.ServiceProviderConfig{
			Type: "sbx",
		},
	}
	variables, err := svc.(*composeService).executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "up", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["IMAGE"], "proj-api")
	assert.Equal(t, variables.prefixed["HAS_BUILD"], "false")
}

// TestHelperProviderConfigImage is not a test: it is the fake provider
// process spawned by TestExecutePlugin_GetServiceConfigResolvesBuildOnlyImage.
func TestHelperProviderConfigImage(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetServiceConfigResolvesBuildOnlyImage")
	}
	responses := json.NewDecoder(os.Stdin)
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	emit(JsonMessage{Type: GetServiceConfigType})
	var config struct {
		Image string          `json:"image"`
		Build json.RawMessage `json:"build"`
	}
	if err := responses.Decode(&config); err != nil {
		emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("reading service config: %v", err)})
		os.Exit(0)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "IMAGE=" + config.Image})
	emit(JsonMessage{Type: SetEnvType, Message: fmt.Sprintf("HAS_BUILD=%t", config.Build != nil)})
	os.Exit(0)
}

// TestExecutePlugin_GetRelayInfo runs executePlugin against a fake provider
// (this test binary re-executed, see TestHelperProviderRelayInfo): the
// get-relay-info message must be answered with one JSON line naming the
// service's dedicated relay-link network — created on demand — with its
// engine-assigned gateway. Never one of the project's own declared
// networks (proj_backend here): those are joined by the dependent too, so
// announcing one of them would give the provider an address every sibling
// on it can also reach.
func TestExecutePlugin_GetRelayInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	// a standalone engine: no Desktop label, so relayInfo converges the
	// dedicated relay-link network instead of announcing the host's loopback
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(client.SystemInfoResult{}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "proj_db_relay", gomock.Any()).
		Return(client.NetworkCreateResult{}, nil)
	inspect := client.NetworkInspectResult{}
	inspect.Network.Name = "proj_db_relay"
	inspect.Network.IPAM.Config = []network.IPAMConfig{
		{Gateway: netip.MustParseAddr("172.18.0.1")},
	}
	apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_db_relay", gomock.Any()).
		Return(inspect, nil)

	// assignment style: Networks is a field promoted from the embedded
	// ContainerSpec, which struct literals cannot set before go1.27
	app := types.ServiceConfig{Name: "app"}
	app.DependsOn = types.DependsOnConfig{"db": types.ServiceDependency{}}
	app.Networks = map[string]*types.ServiceNetworkConfig{"backend": nil}
	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"backend": types.NetworkConfig{Name: "proj_backend"},
		},
		Services: types.Services{
			"db":  {Name: "db", Provider: &types.ServiceProviderConfig{Type: "fake"}},
			"app": app,
		},
	}
	service := project.Services["db"]

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderRelayInfo")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	variables, err := svc.(*composeService).executePlugin(t.Context(), project, cmd, "up", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["RELAY_NETWORK"], "proj_db_relay")
	assert.Equal(t, variables.prefixed["RELAY_GATEWAY"], "172.18.0.1")
}

// TestHelperProviderRelayInfo is not a test: it is the fake provider process
// spawned by TestExecutePlugin_GetRelayInfo. It requests the relay info and
// reports what it received.
func TestHelperProviderRelayInfo(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetRelayInfo")
	}
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	emit(JsonMessage{Type: GetRelayInfoType})
	var answer struct {
		Networks []struct {
			Name    string `json:"name"`
			Gateway string `json:"gateway"`
		} `json:"networks"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&answer); err != nil || len(answer.Networks) > 1 {
		emit(JsonMessage{Type: ErrorType, Message: "bad relay-info answer"})
		os.Exit(1)
	}
	// zero entries (the relay-link network itself could not be resolved) is
	// a valid best-effort answer, not a protocol error — report empty
	// values rather than failing, same as an entry with an empty gateway.
	var name, gateway string
	if len(answer.Networks) == 1 {
		name, gateway = answer.Networks[0].Name, answer.Networks[0].Gateway
	}
	emit(JsonMessage{Type: SetEnvType, Message: "RELAY_NETWORK=" + name})
	emit(JsonMessage{Type: SetEnvType, Message: "RELAY_GATEWAY=" + gateway})
	os.Exit(0)
}

// TestExecutePlugin_GetRelayInfoUnresolvedGateway covers relayInfo's
// best-effort contract (relay.go): the relay-link network's IPAM carrying
// no valid IPv4 gateway is not an error — the network is still announced,
// Gateway simply left empty, rather than dropped or defaulted to
// something wrong. Failing to resolve the network's gateway AT ALL
// (NetworkInspect itself erroring) is the one case relayInfo cannot
// recover a name for either, so the answer carries no entry.
func TestExecutePlugin_GetRelayInfoUnresolvedGateway(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(apiClient *mocks.MockAPIClient)
		wantNetwork string
		wantEntries int
	}{
		{
			name: "NetworkInspect fails",
			setup: func(apiClient *mocks.MockAPIClient) {
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_db_relay", gomock.Any()).
					Return(client.NetworkInspectResult{}, notFoundError{})
			},
			wantEntries: 0,
		},
		{
			name: "IPAM config has an IPv6-only gateway",
			setup: func(apiClient *mocks.MockAPIClient) {
				inspect := client.NetworkInspectResult{}
				inspect.Network.Name = "proj_db_relay"
				inspect.Network.IPAM.Config = []network.IPAMConfig{
					// IPv6-only: valid address, but cfg.Gateway.Is4() is false
					{Gateway: netip.MustParseAddr("fe80::1")},
				}
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_db_relay", gomock.Any()).
					Return(inspect, nil)
			},
			wantNetwork: "proj_db_relay",
			wantEntries: 1,
		},
		{
			name: "IPAM config has no gateway at all",
			setup: func(apiClient *mocks.MockAPIClient) {
				inspect := client.NetworkInspectResult{}
				inspect.Network.Name = "proj_db_relay"
				inspect.Network.IPAM.Config = []network.IPAMConfig{
					// zero-value Gateway: cfg.Gateway.IsValid() is false
					{},
				}
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_db_relay", gomock.Any()).
					Return(inspect, nil)
			},
			wantNetwork: "proj_db_relay",
			wantEntries: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			cli := mocks.NewMockCli(mockCtrl)
			apiClient := mocks.NewMockAPIClient(mockCtrl)
			cli.EXPECT().Client().Return(apiClient).AnyTimes()
			svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
			assert.NilError(t, err)

			// standalone engine: converges the dedicated relay-link network
			apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(client.SystemInfoResult{}, nil)
			apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)
			apiClient.EXPECT().NetworkCreate(gomock.Any(), "proj_db_relay", gomock.Any()).
				Return(client.NetworkCreateResult{}, nil)
			tc.setup(apiClient)

			app := types.ServiceConfig{Name: "app"}
			app.DependsOn = types.DependsOnConfig{"db": types.ServiceDependency{}}
			app.Networks = map[string]*types.ServiceNetworkConfig{"backend": nil}
			project := &types.Project{
				Name: "proj",
				Networks: types.Networks{
					"backend": types.NetworkConfig{Name: "proj_backend"},
				},
				Services: types.Services{
					"db":  {Name: "db", Provider: &types.ServiceProviderConfig{Type: "fake"}},
					"app": app,
				},
			}
			service := project.Services["db"]

			cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderRelayInfo")
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

			variables, err := svc.(*composeService).executePlugin(t.Context(), project, cmd, "up", service)
			assert.NilError(t, err)
			// exactly the two expected vars: nothing silently dropped from the answer
			assert.Equal(t, len(variables.prefixed), 2)
			assert.Equal(t, variables.prefixed["RELAY_NETWORK"], tc.wantNetwork)
			// no gateway to bind to, whether the network was announced or not
			assert.Equal(t, variables.prefixed["RELAY_GATEWAY"], "")
		})
	}
}

// Under Docker Desktop the networks live inside the VM: get-relay-info
// announces the host's own loopback — the address a host process binds to be
// reached through the Desktop proxy — and never inspects the networks.
func TestExecutePlugin_GetRelayInfoDesktop(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	info := client.SystemInfoResult{}
	info.Info.Labels = []string{desktop.EngineLabel + "=unix:///dd.sock"}
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(info, nil)

	app := types.ServiceConfig{Name: "app"}
	app.DependsOn = types.DependsOnConfig{"db": types.ServiceDependency{}}
	app.Networks = map[string]*types.ServiceNetworkConfig{"backend": nil}
	project := &types.Project{
		Name: "proj",
		Networks: types.Networks{
			"backend": types.NetworkConfig{Name: "proj_backend"},
		},
		Services: types.Services{
			"db":  {Name: "db", Provider: &types.ServiceProviderConfig{Type: "fake"}},
			"app": app,
		},
	}
	service := project.Services["db"]

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderRelayInfo")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	variables, err := svc.(*composeService).executePlugin(t.Context(), project, cmd, "up", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["RELAY_NETWORK"], "desktop")
	assert.Equal(t, variables.prefixed["RELAY_GATEWAY"], "127.0.0.1")
}
