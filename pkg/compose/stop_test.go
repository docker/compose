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
	"context"
	"strings"
	"testing"
	"time"

	compose "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestStopTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	ctx := context.Background()
	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt()).Return(
		[]moby.Container{
			testContainer("service1", "123", false),
			testContainer("service1", "456", false),
			testContainer("service2", "789", false),
		}, nil)

	timeout := time.Duration(2) * time.Second
	api.EXPECT().ContainerStop(gomock.Any(), "123", &timeout).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "456", &timeout).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", &timeout).Return(nil)

	err := tested.Stop(ctx, &types.Project{
		Name: strings.ToLower(testProject),
		Services: []types.ServiceConfig{
			{Name: "service1"},
			{Name: "service2"},
		},
	}, compose.StopOptions{
		Timeout: &timeout,
	})
	assert.NilError(t, err)
}
