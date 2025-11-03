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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetModelVariables(t *testing.T) {
	tests := []struct {
		name                string
		modelRef            string
		expectedModelVar    string
		expectedEndpointVar string
	}{
		{
			name:                "simple name with underscore",
			modelRef:            "ai_runner",
			expectedModelVar:    "AI_RUNNER_MODEL",
			expectedEndpointVar: "AI_RUNNER_URL",
		},
		{
			name:                "name with hyphens",
			modelRef:            "ai-runner",
			expectedModelVar:    "AI_RUNNER_MODEL",
			expectedEndpointVar: "AI_RUNNER_URL",
		},
		{
			name:                "complex name with multiple hyphens",
			modelRef:            "my-llm-engine",
			expectedModelVar:    "MY_LLM_ENGINE_MODEL",
			expectedEndpointVar: "MY_LLM_ENGINE_URL",
		},
		{
			name:                "single word",
			modelRef:            "model",
			expectedModelVar:    "MODEL_MODEL",
			expectedEndpointVar: "MODEL_URL",
		},
		{
			name:                "mixed case",
			modelRef:            "AiRunner",
			expectedModelVar:    "AIRUNNER_MODEL",
			expectedEndpointVar: "AIRUNNER_URL",
		},
		{
			name:                "mixed case with hyphens",
			modelRef:            "Ai-Runner",
			expectedModelVar:    "AI_RUNNER_MODEL",
			expectedEndpointVar: "AI_RUNNER_URL",
		},
		{
			name:                "already uppercase with underscores",
			modelRef:            "AI_RUNNER",
			expectedModelVar:    "AI_RUNNER_MODEL",
			expectedEndpointVar: "AI_RUNNER_URL",
		},
		{
			name:                "lowercase simple",
			modelRef:            "airunner",
			expectedModelVar:    "AIRUNNER_MODEL",
			expectedEndpointVar: "AIRUNNER_URL",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				modelVar, endpointVar := GetModelVariables(tt.modelRef)
				assert.Equal(t, tt.expectedModelVar, modelVar, "modelVariable mismatch")
				assert.Equal(t, tt.expectedEndpointVar, endpointVar, "endpointVariable mismatch")
			},
		)
	}
}
