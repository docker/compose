/*
   Copyright 2026 Docker Compose CLI authors

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

package bridge

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func TestLoadAdditionalResources_BuildOnlySkipsPull(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	dockerCLI := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	dockerCLI.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().ImageInspect(gomock.Any(), "test-api").
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"api": {
				Name:   "api",
				Build:  &types.BuildConfig{Context: "."},
				Expose: []string{"8080"},
			},
		},
	}

	actual, err := LoadAdditionalResources(t.Context(), dockerCLI, project)
	assert.NilError(t, err)
	assert.Equal(t, actual.Services["api"].Image, "test-api")
	assert.DeepEqual(t, actual.Services["api"].Expose, types.StringOrNumberList{"8080"})
}
