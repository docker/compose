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

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
)

func TestPrepareProjectForBuild(t *testing.T) {
	t.Run("build service platform", func(t *testing.T) {
		project := types.Project{
			Services: []types.ServiceConfig{
				{
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

		s := &composeService{}
		_, err := s.prepareProjectForBuild(&project, nil)
		assert.NilError(t, err)
		assert.DeepEqual(t, project.Services[0].Build.Platforms, types.StringList{"alice/32"})
	})

	t.Run("build DOCKER_DEFAULT_PLATFORM", func(t *testing.T) {
		project := types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "linux/amd64",
			},
			Services: []types.ServiceConfig{
				{
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

		s := &composeService{}
		_, err := s.prepareProjectForBuild(&project, nil)
		assert.NilError(t, err)
		assert.DeepEqual(t, project.Services[0].Build.Platforms, types.StringList{"linux/amd64"})
	})

	t.Run("skip existing image", func(t *testing.T) {
		project := types.Project{
			Services: []types.ServiceConfig{
				{
					Name:  "test",
					Image: "foo",
					Build: &types.BuildConfig{
						Context: ".",
					},
				},
			},
		}

		s := &composeService{}
		_, err := s.prepareProjectForBuild(&project, map[string]string{"foo": "exists"})
		assert.NilError(t, err)
		assert.Check(t, project.Services[0].Build == nil)
	})

	t.Run("unsupported build platform", func(t *testing.T) {
		project := types.Project{
			Environment: map[string]string{
				"DOCKER_DEFAULT_PLATFORM": "commodore/64",
			},
			Services: []types.ServiceConfig{
				{
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

		s := &composeService{}
		_, err := s.prepareProjectForBuild(&project, nil)
		assert.Check(t, err != nil)
	})
}
