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

package run

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/compose-cli/api/containers"
)

var (
	// AzureNameRegex is used to validate container names
	// Regex was taken from server side error:
	// The container name must contain no more than 63 characters and must match the regex '[a-z0-9]([-a-z0-9]*[a-z0-9])?' (e.g. 'my-name').
	AzureNameRegex = regexp.MustCompile("[a-z0-9]([-a-z0-9]*[a-z0-9])")
)

// TestAzureRandomName ensures compliance with Azure naming requirements
func TestAzureRandomName(t *testing.T) {
	n := getRandomName()
	assert.Assert(t, len(n) < 64)
	assert.Assert(t, len(n) > 1)
	assert.Assert(t, cmp.Regexp(AzureNameRegex, n))
}

func TestPortParse(t *testing.T) {
	testCases := []struct {
		in       string
		expected []containers.Port
	}{
		{
			in: "80",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "80:80",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "80:80/udp",
			expected: []containers.Port{
				{
					ContainerPort: 80,
					HostPort:      80,
					Protocol:      "udp",
				},
			},
		},
		{
			in: "8080:80",
			expected: []containers.Port{
				{
					HostPort:      8080,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
			},
		},
		{
			in: "192.168.0.2:8080:80",
			expected: []containers.Port{
				{
					HostPort:      8080,
					ContainerPort: 80,
					Protocol:      "tcp",
					HostIP:        "192.168.0.2",
				},
			},
		},
		{
			in: "80-81:80-81",
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					Protocol:      "tcp",
				},
				{
					HostPort:      81,
					ContainerPort: 81,
					Protocol:      "tcp",
				},
			},
		},
	}

	for _, testCase := range testCases {
		opts := Opts{
			Publish: []string{testCase.in},
		}
		result, err := opts.toPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, result, testCase.expected, cmpopts.SortSlices(func(x, y containers.Port) bool {
			return x.ContainerPort < y.ContainerPort
		}))
	}
}

func TestLabels(t *testing.T) {
	testCases := []struct {
		in            []string
		expected      map[string]string
		expectedError error
	}{
		{
			in: []string{"label=value"},
			expected: map[string]string{
				"label": "value",
			},
			expectedError: nil,
		},
		{
			in: []string{"label=value", "label=value2"},
			expected: map[string]string{
				"label": "value2",
			},
			expectedError: nil,
		},
		{
			in: []string{"label=value", "label2=value2"},
			expected: map[string]string{
				"label":  "value",
				"label2": "value2",
			},
			expectedError: nil,
		},
		{
			in:            []string{"label"},
			expected:      nil,
			expectedError: errors.New(`wrong label format "label"`),
		},
	}

	for _, testCase := range testCases {
		result, err := toLabels(testCase.in)
		if testCase.expectedError == nil {
			assert.NilError(t, err)
		} else {
			assert.Error(t, err, testCase.expectedError.Error())
		}
		assert.DeepEqual(t, result, testCase.expected)
	}
}

func TestValidateRestartPolicy(t *testing.T) {
	testCases := []struct {
		in            string
		expected      string
		expectedError error
	}{
		{
			in:            "none",
			expected:      "none",
			expectedError: nil,
		},
		{
			in:            "any",
			expected:      "any",
			expectedError: nil,
		},
		{
			in:            "on-failure",
			expected:      "on-failure",
			expectedError: nil,
		},
		{
			in:            "",
			expected:      "none",
			expectedError: nil,
		},
		{
			in:            "no",
			expected:      "none",
			expectedError: nil,
		},
		{
			in:            "always",
			expected:      "any",
			expectedError: nil,
		},

		{
			in:            "toto",
			expected:      "",
			expectedError: errors.New("invalid restart value, must be one of no, always, on-failure"),
		},
	}
	for _, testCase := range testCases {
		result, err := toRestartPolicy(testCase.in)
		if testCase.expectedError == nil {
			assert.NilError(t, err)
		} else {
			assert.Error(t, err, testCase.expectedError.Error())
		}
		assert.Equal(t, testCase.expected, result)
	}
}

func TestToHealthcheck(t *testing.T) {
	testOpt := Opts{
		HealthCmd: "curl",
	}

	assert.DeepEqual(t, testOpt.toHealthcheck(), containers.Healthcheck{
		Disable: false,
		Test:    []string{"curl"},
	})

	testOpt = Opts{
		HealthCmd:         "curl",
		HealthRetries:     3,
		HealthInterval:    5 * time.Second,
		HealthTimeout:     2 * time.Second,
		HealthStartPeriod: 10 * time.Second,
	}

	assert.DeepEqual(t, testOpt.toHealthcheck(), containers.Healthcheck{
		Disable:     false,
		Test:        []string{"curl"},
		Retries:     3,
		Interval:    types.Duration(5 * time.Second),
		StartPeriod: types.Duration(10 * time.Second),
		Timeout:     types.Duration(2 * time.Second),
	})
}
