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
	"testing"

	"gotest.tools/assert"
)

func Test_EnvResolverWithCase(t *testing.T) {
	tests := []struct {
		name            string
		environment     map[string]string
		caseInsensitive bool
		search          string
		expectedValue   string
		expectedOk      bool
	}{
		{
			name: "case sensitive/case match",
			environment: map[string]string{
				"Env1": "Value1",
				"Env2": "Value2",
			},
			caseInsensitive: false,
			search:          "Env1",
			expectedValue:   "Value1",
			expectedOk:      true,
		},
		{
			name: "case sensitive/case unmatch",
			environment: map[string]string{
				"Env1": "Value1",
				"Env2": "Value2",
			},
			caseInsensitive: false,
			search:          "ENV1",
			expectedValue:   "",
			expectedOk:      false,
		},
		{
			name:            "case sensitive/nil environment",
			environment:     nil,
			caseInsensitive: false,
			search:          "Env1",
			expectedValue:   "",
			expectedOk:      false,
		},
		{
			name: "case insensitive/case match",
			environment: map[string]string{
				"Env1": "Value1",
				"Env2": "Value2",
			},
			caseInsensitive: true,
			search:          "Env1",
			expectedValue:   "Value1",
			expectedOk:      true,
		},
		{
			name: "case insensitive/case unmatch",
			environment: map[string]string{
				"Env1": "Value1",
				"Env2": "Value2",
			},
			caseInsensitive: true,
			search:          "ENV1",
			expectedValue:   "Value1",
			expectedOk:      true,
		},
		{
			name: "case insensitive/unmatch",
			environment: map[string]string{
				"Env1": "Value1",
				"Env2": "Value2",
			},
			caseInsensitive: true,
			search:          "Env3",
			expectedValue:   "",
			expectedOk:      false,
		},
		{
			name:            "case insensitive/nil environment",
			environment:     nil,
			caseInsensitive: true,
			search:          "Env1",
			expectedValue:   "",
			expectedOk:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := envResolverWithCase(test.environment, test.caseInsensitive)
			v, ok := f(test.search)
			assert.Equal(t, v, test.expectedValue)
			assert.Equal(t, ok, test.expectedOk)
		})
	}
}
