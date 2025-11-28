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
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"gotest.tools/v3/assert"
)

// mockEventProcessor is a simple mock implementation of api.EventProcessor for testing
type mockEventProcessor struct{}

func (m *mockEventProcessor) Start(ctx context.Context, operation string) {}
func (m *mockEventProcessor) On(events ...api.Resource)                   {}
func (m *mockEventProcessor) Done(operation string, success bool)         {}

func TestConfigureModel_RejectsRuntimeFlags(t *testing.T) {
	tests := []struct {
		name         string
		config       types.ModelConfig
		expectError  bool
		errorMessage string
	}{
		{
			name: "rejects config with runtime flags",
			config: types.ModelConfig{
				Name:         "test-model",
				Model:        "llama3:latest",
				RuntimeFlags: []string{"--flag1", "value1"},
			},
			expectError:  true,
			errorMessage: "runtime flags are not supported for model configuration",
		},
		{
			name: "rejects config with single runtime flag",
			config: types.ModelConfig{
				Name:         "test-model",
				Model:        "llama3:latest",
				RuntimeFlags: []string{"--verbose"},
			},
			expectError:  true,
			errorMessage: "runtime flags are not supported for model configuration",
		},
		{
			name: "accepts config without runtime flags",
			config: types.ModelConfig{
				Name:         "test-model",
				Model:        "llama3:latest",
				RuntimeFlags: nil,
			},
			expectError: false,
		},
		{
			name: "accepts config with empty runtime flags",
			config: types.ModelConfig{
				Name:         "test-model",
				Model:        "llama3:latest",
				RuntimeFlags: []string{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal modelAPI instance
			modelApi := &modelAPI{
				path:    "/usr/local/bin/docker-model",
				prepare: func(ctx context.Context, cmd *exec.Cmd) error { return nil },
				cleanup: func() {},
			}

			// Create a mock event processor
			events := &mockEventProcessor{}

			// Call ConfigureModel
			err := modelApi.ConfigureModel(context.Background(), tt.config, events)

			if tt.expectError {
				assert.ErrorContains(t, err, tt.errorMessage)
			} else if err != nil {
				// For success cases, verify we did NOT get the RuntimeFlags validation error
				// The function may still fail due to exec.Command not being mocked, but it
				// should not fail with the RuntimeFlags validation error
				assert.Assert(t, !strings.Contains(err.Error(), "runtime flags are not supported"),
					"should not fail with RuntimeFlags validation error when RuntimeFlags is empty/nil, got: %v", err)
			}
		})
	}
}
