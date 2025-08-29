/*
   Copyright 2024 Docker Compose CLI authors

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

	"github.com/moby/moby/api/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/filters"
	"github.com/moby/moby/api/types/image"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v2/pkg/api"
)

func TestImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}

	ctx := context.Background()
	args := filters.NewArgs(projectFilter(strings.ToLower(testProject)))
	listOpts := container.ListOptions{All: true, Filters: args}
	api.EXPECT().ServerVersion(gomock.Any()).Return(types.Version{APIVersion: "1.96"}, nil).AnyTimes()
	timeStr1 := "2025-06-06T06:06:06.000000000Z"
	created1, _ := time.Parse(time.RFC3339Nano, timeStr1)
	timeStr2 := "2025-03-03T03:03:03.000000000Z"
	created2, _ := time.Parse(time.RFC3339Nano, timeStr2)
	image1 := imageInspect("image1", "foo:1", 12345, timeStr1)
	image2 := imageInspect("image2", "bar:2", 67890, timeStr2)
	api.EXPECT().ImageInspect(anyCancellableContext(), "foo:1").Return(image1, nil).MaxTimes(2)
	api.EXPECT().ImageInspect(anyCancellableContext(), "bar:2").Return(image2, nil)
	c1 := containerDetail("service1", "123", "running", "foo:1")
	c2 := containerDetail("service1", "456", "running", "bar:2")
	c2.Ports = []container.PortSummary{{PublicPort: 80, PrivatePort: 90, IP: "localhost"}}
	c3 := containerDetail("service2", "789", "exited", "foo:1")
	api.EXPECT().ContainerList(ctx, listOpts).Return([]container.Summary{c1, c2, c3}, nil)

	images, err := tested.Images(ctx, strings.ToLower(testProject), compose.ImagesOptions{})

	expected := map[string]compose.ImageSummary{
		"123": {
			ID:         "image1",
			Repository: "foo",
			Tag:        "1",
			Size:       12345,
			Created:    created1,
		},
		"456": {
			ID:         "image2",
			Repository: "bar",
			Tag:        "2",
			Size:       67890,
			Created:    created2,
		},
		"789": {
			ID:         "image1",
			Repository: "foo",
			Tag:        "1",
			Size:       12345,
			Created:    created1,
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, images, expected)
}

func imageInspect(id string, imageReference string, size int64, created string) image.InspectResponse {
	return image.InspectResponse{
		ID: id,
		RepoTags: []string{
			"someRepo:someTag",
			imageReference,
		},
		Size:    size,
		Created: created,
	}
}

func containerDetail(service string, id string, status string, imageName string) container.Summary {
	return container.Summary{
		ID:     id,
		Names:  []string{"/" + id},
		Image:  imageName,
		Labels: containerLabels(service, false),
		State:  status,
	}
}
