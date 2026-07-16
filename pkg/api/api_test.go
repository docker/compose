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

package api

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestRunOptionsEnvironmentMap(t *testing.T) {
	opts := RunOptions{
		Environment: []string{
			"FOO=BAR",
			"ZOT=",
			"QIX",
		},
	}
	env := types.NewMappingWithEquals(opts.Environment)
	assert.Equal(t, *env["FOO"], "BAR")
	assert.Equal(t, *env["ZOT"], "")
	assert.Check(t, env["QIX"] == nil)
}

func TestGetDependentImages(t *testing.T) {
	const projectName = "demo"
	tests := []struct {
		name     string
		service  types.ServiceConfig
		expected []string
	}{
		{
			name:     "no hooks",
			service:  types.ServiceConfig{Image: "alpine:3.20"},
			expected: nil,
		},
		{
			name: "pre_start hook with explicit image",
			service: types.ServiceConfig{
				Image: "alpine:3.20",
				PreStart: []types.ServiceHook{
					{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "init"}},
				},
			},
			expected: []string{"alpine:3.19"},
		},
		{
			name: "pre_start hook without image is ignored",
			service: types.ServiceConfig{
				Image: "alpine:3.20",
				PreStart: []types.ServiceHook{
					{Image: "busybox", Command: types.ShellCommand{"echo", "a"}},
					{Command: types.ShellCommand{"echo", "b"}},
				},
			},
			expected: []string{"busybox"},
		},
		{
			name: "pre_start hook reusing the service image is ignored",
			service: types.ServiceConfig{
				Image: "alpine:3.20",
				PreStart: []types.ServiceHook{
					{Image: "alpine:3.20", Command: types.ShellCommand{"echo", "same"}},
					{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "other"}},
				},
			},
			expected: []string{"alpine:3.19"},
		},
		{
			name: "pre_start hook reusing the default (build) image name is ignored",
			service: types.ServiceConfig{
				Name:  "web",
				Build: &types.BuildConfig{Context: "."},
				PreStart: []types.ServiceHook{
					{Image: "demo-web", Command: types.ShellCommand{"echo", "same"}},
				},
			},
			expected: nil,
		},
		{
			name: "post_start and pre_stop hooks are not collected",
			service: types.ServiceConfig{
				Image:     "alpine:3.20",
				PostStart: []types.ServiceHook{{Image: "ignored:post", Command: types.ShellCommand{"echo"}}},
				PreStop:   []types.ServiceHook{{Image: "ignored:stop", Command: types.ShellCommand{"echo"}}},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, GetDependentImages(tt.service, projectName), tt.expected)
		})
	}
}
