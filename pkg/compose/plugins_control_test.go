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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

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
	variables, err := svc.(*composeService).executePlugin(cmd, "up", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["TEMPLATE"], "agent:latest")
	// the channel stays usable for more than one request
	assert.Equal(t, variables.prefixed["TEMPLATE_AGAIN"], "agent:latest")
	assert.Equal(t, variables.hosts["db"], "host-gateway")
}

// addhost entries reach dependent services as extra_hosts; setenv variables
// as prefixed environment.
func TestInjectPluginVariables(t *testing.T) {
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "sbx"}}
	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"db": db,
			"app": {
				Name:        "app",
				DependsOn:   types.DependsOnConfig{"db": {}},
				Environment: types.MappingWithEquals{},
			},
			"other": {
				Name:        "other",
				Environment: types.MappingWithEquals{},
			},
		},
	}
	// the create/up path materializes the maps injection mutates before the
	// plan copies services; the injection relies on that sharing
	prepareProviderInjection(project)
	injectPluginVariables(project, db, pluginVariables{
		prefixed: types.Mapping{"PORT": "5734"},
		raw:      types.Mapping{},
		hosts:    types.Mapping{"db": "host-gateway"},
	})

	app := project.Services["app"]
	assert.Equal(t, *app.Environment["DB_PORT"], "5734")
	assert.DeepEqual(t, app.ExtraHosts["db"], []string{"host-gateway"})

	other := project.Services["other"]
	assert.Assert(t, other.ExtraHosts == nil)
	_, injected := other.Environment["DB_PORT"]
	assert.Assert(t, !injected)
}

// TestHelperProviderConfig is not a test: it is the fake provider process
// spawned by TestExecutePlugin_GetServiceConfig.
func TestHelperProviderConfig(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetServiceConfig")
	}
	stdin := bufio.NewReader(os.Stdin)
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	getServiceConfig := func() (template string, ok bool) {
		emit(JsonMessage{Type: GetServiceConfigType})
		line, err := stdin.ReadBytes('\n')
		if err != nil {
			emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("reading service config: %v", err)})
			return "", false
		}
		var config struct {
			Provider struct {
				Options map[string][]string `json:"options"`
			} `json:"provider"`
		}
		if err := json.Unmarshal(line, &config); err != nil {
			emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("bad service config %s: %v", line, err)})
			return "", false
		}
		return config.Provider.Options["template"][0], true
	}

	template, ok := getServiceConfig()
	if !ok {
		os.Exit(0)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "TEMPLATE=" + template})
	emit(JsonMessage{Type: AddHostType, Message: "db=host-gateway"})
	if template, ok = getServiceConfig(); ok {
		emit(JsonMessage{Type: SetEnvType, Message: "TEMPLATE_AGAIN=" + template})
	}
	os.Exit(0)
}
