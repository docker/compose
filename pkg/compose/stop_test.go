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

	"github.com/docker/compose/v2/pkg/utils"

	compose "github.com/docker/compose/v2/pkg/api"
	containerType "github.com/docker/docker/api/types/container"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestStopTimeout(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}

	ctx := context.Background()
	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(
		[]moby.Container{
			testContainer("service1", "123", false),
			testContainer("service1", "456", false),
			testContainer("service2", "789", false),
		}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		volume.ListOptions{
			Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject))),
		}).
		Return(volume.ListResponse{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), network.ListOptions{Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject)))}).
		Return([]network.Summary{}, nil)

	timeout := 2 * time.Second
	stopConfig := containerType.StopOptions{Timeout: utils.DurationSecondToInt(&timeout)}
	api.EXPECT().ContainerStop(gomock.Any(), "123", stopConfig).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "456", stopConfig).Return(nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", stopConfig).Return(nil)

	err := tested.Stop(ctx, strings.ToLower(testProject), compose.StopOptions{
		Timeout: &timeout,
	})
	assert.NilError(t, err)
}
