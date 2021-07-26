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

package mobycli

import (
	"testing"

	"github.com/docker/docker/registry"
	"gotest.tools/assert"
)

func TestIsUsingDefaultRegistry(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		expected bool
	}{
		{
			"without flags",
			[]string{"login"},
			true,
		},
		{
			"login with flags",
			[]string{"login", "-u", "test", "-p", "testpass"},
			true,
		},
		{
			"login to default registry",
			[]string{"login", registry.IndexServer},
			true,
		},
		{
			"login to different registry",
			[]string{"login", "registry.example.com"},
			false,
		},
		{
			"login with flags to different registry",
			[]string{"login", "-u", "test", "-p", "testpass", "registry.example.com"},
			false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := isUsingDefaultRegistry(testCase.input)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestIsUsingPassword(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			"regular password",
			"mypass",
			true,
		},
		{
			"no password or sso",
			"",
			false,
		},
		{
			"personal access token",
			"1508b8bd-b80c-452d-9a7a-ee5607c41bcd",
			false,
		},
		{
			"prefixed personal access token",
			"dckrp_ee5607c41bcd",
			false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := isUsingPassword(testCase.input)
			assert.Equal(t, testCase.expected, result)
		})
	}
}
