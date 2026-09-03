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
