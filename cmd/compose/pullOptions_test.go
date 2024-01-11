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
	"gotest.tools/v3/assert"
)

func TestApplyPullOptions(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"must-build": {
				Name: "must-build",
				// No image, local build only
				Build: &types.BuildConfig{
					Context: ".",
				},
			},
			"has-build": {
				Name:  "has-build",
				Image: "registry.example.com/myservice",
				Build: &types.BuildConfig{
					Context: ".",
				},
			},
			"must-pull": {
				Name:  "must-pull",
				Image: "registry.example.com/another-service",
			},
		},
	}
	project, err := pullOptions{
		policy: types.PullPolicyMissing,
	}.apply(project, nil)
	assert.NilError(t, err)

	assert.Equal(t, project.Services["must-build"].PullPolicy, "") // still default
	assert.Equal(t, project.Services["has-build"].PullPolicy, types.PullPolicyMissing)
	assert.Equal(t, project.Services["must-pull"].PullPolicy, types.PullPolicyMissing)
}
