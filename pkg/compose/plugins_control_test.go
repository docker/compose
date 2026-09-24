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

// TestExecutePlugin_GetRelayInfo runs executePlugin against a fake provider
// (this test binary re-executed, see TestHelperProviderRelayInfo): the
// get-relay-info message must be answered with one JSON line listing the
// networks the relay would join — the consumers' networks — each with its
// engine-assigned gateway.
func TestExecutePlugin_GetRelayInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	// a standalone engine: no Desktop label, the gateway comes from the
	// network's IPAM
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(client.SystemInfoResult{}, nil)
	inspect := client.NetworkInspectResult{}
	inspect.Network.Name = "proj_backend"
	inspect.Network.IPAM.Config = []network.IPAMConfig{
		{Gateway: netip.MustParseAddr("172.18.0.1")},
	}
	apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_backend", gomock.Any()).
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
	assert.Equal(t, variables.prefixed["RELAY_NETWORK"], "proj_backend")
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
	if err := json.NewDecoder(os.Stdin).Decode(&answer); err != nil || len(answer.Networks) != 1 {
		emit(JsonMessage{Type: ErrorType, Message: "bad relay-info answer"})
		os.Exit(1)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "RELAY_NETWORK=" + answer.Networks[0].Name})
	emit(JsonMessage{Type: SetEnvType, Message: "RELAY_GATEWAY=" + answer.Networks[0].Gateway})
	os.Exit(0)
}

// TestExecutePlugin_GetRelayInfoUnresolvedGateway covers relayInfo's
// best-effort contract (relay.go): when a network's gateway cannot be
// resolved — the inspect itself fails, or the IPAM config carries no valid
// IPv4 gateway — the network is still listed, Gateway simply left empty,
// rather than dropped or defaulted to something wrong.
func TestExecutePlugin_GetRelayInfoUnresolvedGateway(t *testing.T) {
	tests := []struct {
		name  string
		setup func(apiClient *mocks.MockAPIClient)
	}{
		{
			name: "NetworkInspect fails",
			setup: func(apiClient *mocks.MockAPIClient) {
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_backend", gomock.Any()).
					Return(client.NetworkInspectResult{}, notFoundError{})
			},
		},
		{
			name: "IPAM config has an IPv6-only gateway",
			setup: func(apiClient *mocks.MockAPIClient) {
				inspect := client.NetworkInspectResult{}
				inspect.Network.Name = "proj_backend"
				inspect.Network.IPAM.Config = []network.IPAMConfig{
					// IPv6-only: valid address, but cfg.Gateway.Is4() is false
					{Gateway: netip.MustParseAddr("fe80::1")},
				}
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_backend", gomock.Any()).
					Return(inspect, nil)
			},
		},
		{
			name: "IPAM config has no gateway at all",
			setup: func(apiClient *mocks.MockAPIClient) {
				inspect := client.NetworkInspectResult{}
				inspect.Network.Name = "proj_backend"
				inspect.Network.IPAM.Config = []network.IPAMConfig{
					// zero-value Gateway: cfg.Gateway.IsValid() is false
					{},
				}
				apiClient.EXPECT().NetworkInspect(gomock.Any(), "proj_backend", gomock.Any()).
					Return(inspect, nil)
			},
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

			// standalone engine: falls through to the NetworkInspect path
			apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(client.SystemInfoResult{}, nil)
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
			// the network is still announced...
			assert.Equal(t, variables.prefixed["RELAY_NETWORK"], "proj_backend")
			// ...but with no gateway to bind to
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
	assert.Equal(t, variables.prefixed["RELAY_NETWORK"], "proj_backend")
	assert.Equal(t, variables.prefixed["RELAY_GATEWAY"], "127.0.0.1")
}
