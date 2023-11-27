/*
   Copyright 2023 Docker Compose CLI authors

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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/require"
)

func TestApplyPlatforms_InferFromRuntime(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
							"alice/32",
						},
					},
					Platform: "alice/32",
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, true))
		require.EqualValues(t, []string{"alice/32"}, project.Services["test"].Build.Platforms)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, false))
		require.EqualValues(t, []string{"linux/amd64", "linux/arm64", "alice/32"},
			project.Services["test"].Build.Platforms)
	})
}

func TestApplyPlatforms_DockerDefaultPlatform(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "linux/amd64",
			},
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
						},
					},
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, true))
		require.EqualValues(t, []string{"linux/amd64"}, project.Services["test"].Build.Platforms)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.NoError(t, applyPlatforms(project, false))
		require.EqualValues(t, []string{"linux/amd64", "linux/arm64"},
			project.Services["test"].Build.Platforms)
	})
}

func TestApplyPlatforms_UnsupportedPlatform(t *testing.T) {
	makeProject := func() *types.Project {
		return &types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "commodore/64",
			},
			Services: types.Services{
				"test": {
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
						Platforms: []string{
							"linux/amd64",
							"linux/arm64",
						},
					},
				},
			},
		}
	}

	t.Run("SinglePlatform", func(t *testing.T) {
		project := makeProject()
		require.EqualError(t, applyPlatforms(project, true),
			`service "test" build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: commodore/64`)
	})

	t.Run("MultiPlatform", func(t *testing.T) {
		project := makeProject()
		require.EqualError(t, applyPlatforms(project, false),
			`service "test" build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: commodore/64`)
	})
}
