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

package formatter

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/cli/options/run"
)

func TestDisplayPortsNoDomainname(t *testing.T) {
	testCases := []struct {
		name     string
		in       []string
		expected []string
	}{
		{
			name:     "simple",
			in:       []string{"80"},
			expected: []string{"0.0.0.0:80->80/tcp"},
		},
		{
			name:     "different ports",
			in:       []string{"80:90"},
			expected: []string{"0.0.0.0:80->90/tcp"},
		},
		{
			name:     "host ip",
			in:       []string{"192.168.0.1:80:90"},
			expected: []string{"192.168.0.1:80->90/tcp"},
		},
		{
			name:     "port range",
			in:       []string{"80-90:80-90"},
			expected: []string{"0.0.0.0:80-90->80-90/tcp"},
		},
		{
			name:     "grouping",
			in:       []string{"80:80", "81:81"},
			expected: []string{"0.0.0.0:80-81->80-81/tcp"},
		},
		{
			name:     "groups",
			in:       []string{"80:80", "82:82"},
			expected: []string{"0.0.0.0:80->80/tcp", "0.0.0.0:82->82/tcp"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runOpts := run.Opts{
				Publish: testCase.in,
			}
			containerConfig, err := runOpts.ToContainerConfig("test")
			assert.NilError(t, err)

			out := PortsToStrings(containerConfig.Ports, "")
			assert.DeepEqual(t, testCase.expected, out)
		})
	}
}

func TestDisplayPortsWithDomainname(t *testing.T) {
	runOpts := run.Opts{
		Publish: []string{"80"},
	}
	containerConfig, err := runOpts.ToContainerConfig("test")
	assert.NilError(t, err)

	out := PortsToStrings(containerConfig.Ports, "mydomain.westus.azurecontainner.io")
	assert.DeepEqual(t, []string{"mydomain.westus.azurecontainner.io:80->80/tcp"}, out)
}
