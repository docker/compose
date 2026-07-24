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
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

func Test_dockerFilePath(t *testing.T) {
	tests := []struct {
		name       string
		ctxName    string
		dockerfile string
		expected   string
	}{
		{
			name:       "empty dockerfile",
			ctxName:    "/some/local/dir",
			dockerfile: "",
			expected:   "",
		},
		{
			name:       "local dir with relative dockerfile",
			ctxName:    "/some/local/dir",
			dockerfile: "Dockerfile",
			expected:   "/some/local/dir/Dockerfile",
		},
		{
			name:       "local dir with absolute dockerfile",
			ctxName:    "/some/local/dir",
			dockerfile: "/absolute/path/Dockerfile",
			expected:   "/absolute/path/Dockerfile",
		},
		{
			name:       "ssh URL preserves double slash",
			ctxName:    "ssh://git@github.com:22/docker/welcome-to-docker.git",
			dockerfile: "Dockerfile",
			expected:   "Dockerfile",
		},
		{
			name:       "git:// URL returns dockerfile as-is",
			ctxName:    "git://github.com/docker/compose.git",
			dockerfile: "Dockerfile",
			expected:   "Dockerfile",
		},
		{
			name:       "https git URL returns dockerfile as-is",
			ctxName:    "https://github.com/docker/compose.git",
			dockerfile: "Dockerfile",
			expected:   "Dockerfile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dockerFilePath(tt.ctxName, tt.dockerfile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_addBuildDependencies(t *testing.T) {
	project := &types.Project{Services: types.Services{
		"test": types.ServiceConfig{
			Build: &types.BuildConfig{
				AdditionalContexts: map[string]string{
					"foo": "service:foo",
					"bar": "service:bar",
				},
			},
		},
		"foo": types.ServiceConfig{
			Build: &types.BuildConfig{
				AdditionalContexts: map[string]string{
					"zot": "service:zot",
				},
			},
		},
		"bar": types.ServiceConfig{
			Build: &types.BuildConfig{},
		},
		"zot": types.ServiceConfig{
			Build: &types.BuildConfig{},
		},
	}}

	services := addBuildDependencies([]string{"test"}, project)
	expected := []string{"test", "foo", "bar", "zot"}
	slices.Sort(services)
	slices.Sort(expected)
	assert.DeepEqual(t, services, expected)
}

// TestGetLocalImagesDigests_PreStartHook ensures pre_start hook images are
// inspected alongside the service image so they get resolved (see issue #13924).
func TestGetLocalImagesDigests_PreStartHook(t *testing.T) {
	tested, apiClient := newPreStartTestService(t)

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "alpine:3.20",
				PreStart: []types.ServiceHook{
					{Image: "alpine:3.19", Command: types.ShellCommand{"echo", "init"}},
				},
			},
		},
	}

	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine:3.20").
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:service"}}, nil)
	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine:3.19").
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:hook"}}, nil)

	images, err := tested.getLocalImagesDigests(t.Context(), project)
	assert.NilError(t, err)
	assert.Equal(t, images["alpine:3.20"].ID, "sha256:service")
	assert.Equal(t, images["alpine:3.19"].ID, "sha256:hook")
}
