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
	"testing"

	"github.com/docker/docker/api/types/filters"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/local/mocks"

	apitypes "github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestDown(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt()).Return(
		[]apitypes.Container{testContainer("service1", "123"), testContainer("service2", "456"), testContainer("service2", "789"), testContainer("service_orphan", "321")}, nil)

	api.EXPECT().ContainerStop(gomock.Any(), "123", nil).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "456", nil).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", nil).Return(nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "456", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "789", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)

	api.EXPECT().NetworkList(gomock.Any(), apitypes.NetworkListOptions{Filters: filters.NewArgs(projectFilter(testProject))}).Return([]apitypes.NetworkResource{{ID: "myProject_default"}}, nil)

	api.EXPECT().NetworkRemove(gomock.Any(), "myProject_default").Return(nil)

	err := tested.Down(context.Background(), testProject, compose.DownOptions{})
	assert.NilError(t, err)
}

func TestDownRemoveOrphans(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt()).Return(
		[]apitypes.Container{testContainer("service1", "123"), testContainer("service2", "789"), testContainer("service_orphan", "321")}, nil)

	api.EXPECT().ContainerStop(gomock.Any(), "123", nil).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", nil).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "321", nil).Return(nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "789", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "321", apitypes.ContainerRemoveOptions{Force: true}).Return(nil)

	api.EXPECT().NetworkList(gomock.Any(), apitypes.NetworkListOptions{Filters: filters.NewArgs(projectFilter(testProject))}).Return([]apitypes.NetworkResource{{ID: "myProject_default"}}, nil)

	api.EXPECT().NetworkRemove(gomock.Any(), "myProject_default").Return(nil)

	err := tested.Down(context.Background(), testProject, compose.DownOptions{RemoveOrphans: true})
	assert.NilError(t, err)
}
