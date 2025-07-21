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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestDown(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(
		client.ContainerListResult{Items: []container.Summary{
			testContainer("service1", "123", false),
			testContainer("service2", "456", false),
			testContainer("service2", "789", false),
			testContainer("service_orphan", "321", true),
		}}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)

	// network names are not guaranteed to be unique, ensure Compose handles
	// cleanup properly if duplicates are inadvertently created
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{Items: []network.Summary{
			{Network: network.Network{ID: "abc123", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
			{Network: network.Network{ID: "def456", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
		}}, nil)

	stopOptions := client.ContainerStopOptions{}
	api.EXPECT().ContainerStop(gomock.Any(), "123", stopOptions).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "456", stopOptions).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", stopOptions).Return(client.ContainerStopResult{}, nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "456", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "789", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", networkFilter("default")),
	}).Return(client.NetworkListResult{Items: []network.Summary{
		{Network: network.Network{ID: "abc123", Name: "myProject_default"}},
		{Network: network.Network{ID: "def456", Name: "myProject_default"}},
	}}, nil)
	api.EXPECT().NetworkInspect(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkInspectResult{
		Network: network.Inspect{Network: network.Network{ID: "abc123"}},
	}, nil)
	api.EXPECT().NetworkInspect(gomock.Any(), "def456", gomock.Any()).Return(client.NetworkInspectResult{
		Network: network.Inspect{Network: network.Network{ID: "def456"}},
	}, nil)
	api.EXPECT().NetworkRemove(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)
	api.EXPECT().NetworkRemove(gomock.Any(), "def456", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{})
	assert.NilError(t, err)
}

func TestDownWithGivenServices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(client.ContainerListResult{
		Items: []container.Summary{
			testContainer("service1", "123", false),
			testContainer("service2", "456", false),
			testContainer("service2", "789", false),
			testContainer("service_orphan", "321", true),
		},
	}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)

	// network names are not guaranteed to be unique, ensure Compose handles
	// cleanup properly if duplicates are inadvertently created
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{Items: []network.Summary{
			{Network: network.Network{ID: "abc123", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
			{Network: network.Network{ID: "def456", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
		}}, nil)

	api.EXPECT().ContainerStop(gomock.Any(), "123", client.ContainerStopOptions{}).Return(client.ContainerStopResult{}, nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", networkFilter("default")),
	}).Return(client.NetworkListResult{Items: []network.Summary{
		{Network: network.Network{ID: "abc123", Name: "myProject_default"}},
	}}, nil)
	api.EXPECT().NetworkInspect(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkInspectResult{Network: network.Inspect{Network: network.Network{ID: "abc123"}}}, nil)
	api.EXPECT().NetworkRemove(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{
		Services: []string{"service1", "not-running-service"},
	})
	assert.NilError(t, err)
}

func TestDownWithSpecifiedServiceButTheServicesAreNotRunning(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(client.ContainerListResult{
		Items: []container.Summary{
			testContainer("service1", "123", false),
			testContainer("service2", "456", false),
			testContainer("service2", "789", false),
			testContainer("service_orphan", "321", true),
		},
	}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)

	// network names are not guaranteed to be unique, ensure Compose handles
	// cleanup properly if duplicates are inadvertently created
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{Items: []network.Summary{
			{Network: network.Network{ID: "abc123", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
			{Network: network.Network{ID: "def456", Name: "myProject_default", Labels: map[string]string{compose.NetworkLabel: "default"}}},
		}}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{
		Services: []string{"not-running-service1", "not-running-service2"},
	})
	assert.NilError(t, err)
}

func TestDownRemoveOrphans(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(true)).Return(
		client.ContainerListResult{
			Items: []container.Summary{
				testContainer("service1", "123", false),
				testContainer("service2", "789", false),
				testContainer("service_orphan", "321", true),
			},
		}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{
			Items: []network.Summary{{
				Network: network.Network{
					Name:   "myProject_default",
					Labels: map[string]string{compose.NetworkLabel: "default"},
				},
			}},
		}, nil)

	stopOptions := client.ContainerStopOptions{}
	api.EXPECT().ContainerStop(gomock.Any(), "123", stopOptions).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "789", stopOptions).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "321", stopOptions).Return(client.ContainerStopResult{}, nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "789", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "321", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", networkFilter("default")),
	}).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "abc123", Name: "myProject_default"}}},
	}, nil)
	api.EXPECT().NetworkInspect(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkInspectResult{
		Network: network.Inspect{Network: network.Network{ID: "abc123"}},
	}, nil)
	api.EXPECT().NetworkRemove(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{RemoveOrphans: true})
	assert.NilError(t, err)
}

func TestDownRemoveVolumes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(
		client.ContainerListResult{
			Items: []container.Summary{testContainer("service1", "123", false)},
		}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{
			Items: []volume.Volume{{Name: "myProject_volume"}},
		}, nil)
	api.EXPECT().VolumeInspect(gomock.Any(), "myProject_volume", gomock.Any()).
		Return(client.VolumeInspectResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{}, nil)

	api.EXPECT().ContainerStop(gomock.Any(), "123", client.ContainerStopOptions{}).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().VolumeRemove(gomock.Any(), "myProject_volume", client.VolumeRemoveOptions{Force: true}).Return(client.VolumeRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{Volumes: true})
	assert.NilError(t, err)
}

func TestDownRemoveImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	opts := compose.DownOptions{
		Project: &types.Project{
			Name: strings.ToLower(testProject),
			Services: types.Services{
				"local-anonymous":     {Name: "local-anonymous"},
				"local-named":         {Name: "local-named", Image: "local-named-image"},
				"remote":              {Name: "remote", Image: "remote-image"},
				"remote-tagged":       {Name: "remote-tagged", Image: "registry.example.com/remote-image-tagged:v1.0"},
				"no-images-anonymous": {Name: "no-images-anonymous"},
				"no-images-named":     {Name: "no-images-named", Image: "missing-named-image"},
			},
		},
	}

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).
		Return(client.ContainerListResult{
			Items: []container.Summary{
				testContainer("service1", "123", false),
			},
		}, nil).
		AnyTimes()

	api.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("dangling", "false"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{
			Labels:   types.Labels{compose.ServiceLabel: "local-anonymous"},
			RepoTags: []string{"testproject-local-anonymous:latest"},
		},
		{
			Labels:   types.Labels{compose.ServiceLabel: "local-named"},
			RepoTags: []string{"local-named-image:latest"},
		},
	}}, nil).AnyTimes()

	imagesToBeInspected := map[string]bool{
		"testproject-local-anonymous":     true,
		"local-named-image":               true,
		"remote-image":                    true,
		"testproject-no-images-anonymous": false,
		"missing-named-image":             false,
	}
	for img, exists := range imagesToBeInspected {
		var resp image.InspectResponse
		var err error
		if exists {
			resp.RepoTags = []string{img}
		} else {
			err = errdefs.ErrNotFound.WithMessage(fmt.Sprintf("test specified that image %q should not exist", img))
		}

		api.EXPECT().ImageInspect(gomock.Any(), img).
			Return(client.ImageInspectResult{InspectResponse: resp}, err).
			AnyTimes()
	}

	api.EXPECT().ImageInspect(gomock.Any(), "registry.example.com/remote-image-tagged:v1.0").
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{RepoTags: []string{"registry.example.com/remote-image-tagged:v1.0"}}}, nil).
		AnyTimes()

	localImagesToBeRemoved := []string{
		"testproject-local-anonymous:latest",
		"local-named-image:latest",
	}
	for _, img := range localImagesToBeRemoved {
		// test calls down --rmi=local then down --rmi=all, so local images
		// get "removed" 2x, while other images are only 1x
		api.EXPECT().ImageRemove(gomock.Any(), img, client.ImageRemoveOptions{}).
			Return(client.ImageRemoveResult{}, nil).
			Times(2)
	}

	t.Log("-> docker compose down --rmi=local")
	opts.Images = "local"
	err = tested.Down(t.Context(), strings.ToLower(testProject), opts)
	assert.NilError(t, err)

	otherImagesToBeRemoved := []string{
		"remote-image:latest",
		"registry.example.com/remote-image-tagged:v1.0",
	}
	for _, img := range otherImagesToBeRemoved {
		api.EXPECT().ImageRemove(gomock.Any(), img, client.ImageRemoveOptions{}).
			Return(client.ImageRemoveResult{}, nil).
			Times(1)
	}

	t.Log("-> docker compose down --rmi=all")
	opts.Images = "all"
	err = tested.Down(t.Context(), strings.ToLower(testProject), opts)
	assert.NilError(t, err)
}

func TestDownRemoveImages_NoLabel(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	ctr := testContainer("service1", "123", false)

	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).Return(
		client.ContainerListResult{
			Items: []container.Summary{ctr},
		}, nil)

	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{
			Items: []volume.Volume{{Name: "myProject_volume"}},
		}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{}, nil)

	// ImageList returns no images for the project since they were unlabeled
	// (created by an older version of Compose)
	api.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("dangling", "false"),
	}).Return(client.ImageListResult{}, nil)

	api.EXPECT().ImageInspect(gomock.Any(), "testproject-service1", gomock.Any()).Return(client.ImageInspectResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "123", client.ContainerStopOptions{}).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().ImageRemove(gomock.Any(), "testproject-service1:latest", client.ImageRemoveOptions{}).Return(client.ImageRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{Images: "local"})
	assert.NilError(t, err)
}

func prepareMocks(mockCtrl *gomock.Controller) (*mocks.MockAPIClient, *mocks.MockCli) {
	api := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(api).AnyTimes()
	cli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	cli.EXPECT().Out().Return(streams.NewOut(os.Stdout)).AnyTimes()
	return api, cli
}
