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

	containerType "github.com/docker/docker/api/types/container"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v2/pkg/api"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
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
	listOpts := containerType.ListOptions{All: true, Filters: args}
	image1 := imageInspect("image1", "foo:1", 12345)
	image2 := imageInspect("image2", "bar:2", 67890)
	api.EXPECT().ImageInspectWithRaw(anyCancellableContext(), "foo:1").Return(image1, nil, nil)
	api.EXPECT().ImageInspectWithRaw(anyCancellableContext(), "bar:2").Return(image2, nil, nil)
	c1 := containerDetail("service1", "123", "running", "foo:1")
	c2 := containerDetail("service1", "456", "running", "bar:2")
	c2.Ports = []moby.Port{{PublicPort: 80, PrivatePort: 90, IP: "localhost"}}
	c3 := containerDetail("service2", "789", "exited", "foo:1")
	api.EXPECT().ContainerList(ctx, listOpts).Return([]moby.Container{c1, c2, c3}, nil)

	images, err := tested.Images(ctx, strings.ToLower(testProject), compose.ImagesOptions{})

	expected := []compose.ImageSummary{
		{
			ID:            "image1",
			ContainerName: "123",
			Repository:    "foo",
			Tag:           "1",
			Size:          12345,
		},
		{
			ID:            "image2",
			ContainerName: "456",
			Repository:    "bar",
			Tag:           "2",
			Size:          67890,
		},
		{
			ID:            "image1",
			ContainerName: "789",
			Repository:    "foo",
			Tag:           "1",
			Size:          12345,
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, images, expected)
}

func imageInspect(id string, image string, size int64) moby.ImageInspect {
	return moby.ImageInspect{
		ID: id,
		RepoTags: []string{
			"someRepo:someTag",
			image,
		},
		Size: size,
	}
}

func containerDetail(service string, id string, status string, image string) moby.Container {
	return moby.Container{
		ID:     id,
		Names:  []string{"/" + id},
		Image:  image,
		Labels: containerLabels(service, false),
		State:  status,
	}
}
