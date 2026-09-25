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
	"errors"
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

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil)

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

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt("service1")).Return(client.ContainerListResult{}, nil)

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
				runningOneOff("service1", "654"),
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
	// The RUNNING one-off of a declared service goes down too — down stops the
	// application — via the per-service removal loop (it matches isService;
	// isOrphaned deliberately excludes running one-offs so `up` never kills a
	// live session). Exactly one stop+remove.
	api.EXPECT().ContainerStop(gomock.Any(), "654", stopOptions).Return(client.ContainerStopResult{}, nil)

	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "789", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "321", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "654", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", networkFilter("default")),
	}).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "abc123", Name: "myProject_default"}}},
	}, nil)
	api.EXPECT().NetworkInspect(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkInspectResult{
		Network: network.Inspect{Network: network.Network{ID: "abc123"}},
	}, nil)
	api.EXPECT().NetworkRemove(gomock.Any(), "abc123", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil)

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

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil)

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
				"local-named":         {Name: "local-named", ContainerSpec: types.ContainerSpec{Image: "local-named-image"}},
				"remote":              {Name: "remote", ContainerSpec: types.ContainerSpec{Image: "remote-image"}},
				"remote-tagged":       {Name: "remote-tagged", ContainerSpec: types.ContainerSpec{Image: "registry.example.com/remote-image-tagged:v1.0"}},
				"no-images-anonymous": {Name: "no-images-anonymous"},
				"no-images-named":     {Name: "no-images-named", ContainerSpec: types.ContainerSpec{Image: "missing-named-image"}},
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

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil).AnyTimes()

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

	api.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil).AnyTimes()

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

	api.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil)

	api.EXPECT().ImageInspect(gomock.Any(), "testproject-service1", gomock.Any()).Return(client.ImageInspectResult{}, nil)
	api.EXPECT().ContainerStop(gomock.Any(), "123", client.ContainerStopOptions{}).Return(client.ContainerStopResult{}, nil)
	api.EXPECT().ContainerRemove(gomock.Any(), "123", client.ContainerRemoveOptions{Force: true}).Return(client.ContainerRemoveResult{}, nil)

	api.EXPECT().ImageRemove(gomock.Any(), "testproject-service1:latest", client.ImageRemoveOptions{}).Return(client.ImageRemoveResult{}, nil)

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{Images: "local"})
	assert.NilError(t, err)
}

// hookFilterListOpt returns the ContainerListOptions used by removePreStartHookContainers.
// When services are provided the filter is scoped per service; otherwise it matches the
// whole project. The argument order must match the production code: project → service → hook.
func hookFilterListOpt(services ...string) client.ContainerListOptions {
	if len(services) == 0 {
		f := projectFilter(strings.ToLower(testProject))
		f.Add("label", hookFilter(preStartHookType))
		return client.ContainerListOptions{Filters: f, All: true}
	}
	// For service-scoped calls there is one list per service; callers should pass a single service.
	f := projectFilter(strings.ToLower(testProject))
	for _, svc := range services {
		f.Add("label", serviceFilter(svc))
	}
	f.Add("label", hookFilter(preStartHookType))
	return client.ContainerListOptions{Filters: f, All: true}
}

func prepareMocks(mockCtrl *gomock.Controller) (*mocks.MockAPIClient, *mocks.MockCli) {
	api := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(api).AnyTimes()
	cli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	cli.EXPECT().Out().Return(streams.NewOut(os.Stdout)).AnyTimes()
	return api, cli
}

// TestEnsureImagesDown_TaggedImageListingFailureStaysVisibleWithoutAbortingDown
// guards the sibling fix to #14219 found during review: ImagesToPrune's own
// listing call used to run synchronously inside ensureImagesDown and abort
// it on failure, before down() ever scheduled the network/volume ops it had
// already built — the same hazard class fixed for dangling images.
// ensureImagesDown can no longer return an error at all (enforced by its
// signature), so a listing failure can only surface once
// removeTaggedImagesOp actually runs as one of down's concurrently
// scheduled ops, and it must not prevent the sibling dangling-images op
// from running fine on its own.
func TestEnsureImagesDown_TaggedImageListingFailureStaysVisibleWithoutAbortingDown(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	rec := &capturingEvents{}
	svcIface, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	svc := svcIface.(*composeService)

	project := &types.Project{Name: "prj"}
	listErr := errors.New("connection refused")
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{}, listErr)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil)

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	opErr := ops[0]()
	assert.Error(t, opErr, listErr.Error())
	assert.NilError(t, ops[1]()) // dangling-images op is independent, still runs fine

	assert.Equal(t, len(rec.resources), 1)
	assert.Equal(t, rec.resources[0].ID, compose.ResourceCompose)
	assert.Equal(t, rec.resources[0].Status, compose.Error)
	assert.Equal(t, rec.resources[0].Details, listErr.Error())
}

// TestEnsureImagesDown_ReportsDanglingImagesAsOneGroupedEvent guards that
// dangling-image removal is reported as a single grouped event regardless
// of count, instead of one row per meaningless raw image ID.
func TestEnsureImagesDown_ReportsDanglingImagesAsOneGroupedEvent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	rec := &capturingEvents{}
	svcIface, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	svc := svcIface.(*composeService)

	project := &types.Project{Name: "prj"}
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{}, nil)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:aaa"},
		{ID: "sha256:bbb"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:aaa", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:bbb", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrNotFound.WithMessage("already removed"))

	// RemoveOrphans:true bypasses the per-service keep check so this test
	// stays focused on the single-grouped-event behavior.
	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	for _, op := range ops {
		assert.NilError(t, op())
	}

	events := make([]string, len(rec.resources))
	for i, e := range rec.resources {
		events[i] = e.ID + ": " + e.Text
	}
	assert.DeepEqual(t, events, []string{
		"Dangling images: Removing",
		"Dangling images: Removed",
	})
}

// TestEnsureImagesDown_SparesDanglingImagesOfOrphanedServices guards a bug
// caught in review: ImagesToPrune already spares a service's tagged image
// when the service is no longer in the project and RemoveOrphans isn't set;
// its dangling images must be spared the same way, or `down --rmi` leaves
// an inconsistent result (tagged image kept, dangling image gone).
func TestEnsureImagesDown_SparesDanglingImagesOfOrphanedServices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name: "prj",
		Services: types.Services{
			"web": {Name: "web", ContainerSpec: types.ContainerSpec{Image: "web-image"}},
		},
	}
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{}, nil)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:web-dangling", Labels: types.Labels{compose.ServiceLabel: "web"}},
		{ID: "sha256:orphan-dangling", Labels: types.Labels{compose.ServiceLabel: "orphan"}},
	}}, nil)
	// only the known service's dangling image may be removed; a call for
	// the orphaned one is an unexpected call and fails the test
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:web-dangling", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local"})
	for _, op := range ops {
		assert.NilError(t, op())
	}
}

// TestEnsureImagesDown_RemoveOrphansAlsoTakesDanglingImages guards that
// --remove-orphans overrides the spare-orphans behavior for dangling
// images too, matching what it already does for tagged images.
func TestEnsureImagesDown_RemoveOrphansAlsoTakesDanglingImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name: "prj",
		Services: types.Services{
			"web": {Name: "web", ContainerSpec: types.ContainerSpec{Image: "web-image"}},
		},
	}
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{}, nil)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:orphan-dangling", Labels: types.Labels{compose.ServiceLabel: "orphan"}},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:orphan-dangling", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	for _, op := range ops {
		assert.NilError(t, op())
	}
}

// newDanglingImagesFixture sets up a composeService with a capturing event
// recorder and a bare "prj" project, and stubs the tagged-image listing
// (ImagesToPrune's own query) as empty so the tests below can focus solely
// on the dangling-images op. Shared by the three tests guarding the fix for
// #14219.
func newDanglingImagesFixture(t *testing.T) (*mocks.MockAPIClient, *composeService, *capturingEvents, *types.Project) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)

	apiClient, cli := prepareMocks(mockCtrl)
	rec := &capturingEvents{}
	svcIface, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{}, nil)

	return apiClient, svcIface.(*composeService), rec, &types.Project{Name: "prj"}
}

// TestEnsureImagesDown_NoDanglingImagesToRemove guards the fix for #14219: a
// repeat `down --rmi` with nothing left to clean up must stay silent about
// dangling images instead of reporting a misleading "Removed" for a no-op,
// mirroring how removeNetwork stays silent when it finds nothing to do.
func TestEnsureImagesDown_NoDanglingImagesToRemove(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil)

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op
	for _, op := range ops {
		assert.NilError(t, op())
	}

	assert.DeepEqual(t, rec.resources, []compose.Resource(nil))
}

// TestEnsureImagesDown_AllDanglingImagesSparedStaysSilent guards the other
// silent-no-op edge case: the dangling-image list isn't empty, but keep
// spares every entry (an orphaned service no longer in the project,
// RemoveOrphans not set). A non-empty raw list must not be misread as
// "something to remove".
func TestEnsureImagesDown_AllDanglingImagesSparedStaysSilent(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:orphan-dangling", Labels: types.Labels{compose.ServiceLabel: "orphan"}},
	}}, nil)
	// no ImageRemove expectation — a call for the spared image fails the test

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local"})
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op
	for _, op := range ops {
		assert.NilError(t, op())
	}

	assert.DeepEqual(t, rec.resources, []compose.Resource(nil))
}

// TestEnsureImagesDown_DanglingListFailureStaysVisibleWithoutAbortingDown
// guards the fix requested in review of #14219: a failure while checking for
// dangling images must not be reported under the "Dangling images" label
// (we don't yet know whether it would have been a no-op or a real removal),
// must surface as a visible error, and — critically — must not prevent the
// rest of `down` from running: the op reports its own failure and returns
// it, but `ensureImagesDown` itself never aborts because of it, so sibling
// ops (networks, volumes, containers) still get scheduled and run.
func TestEnsureImagesDown_DanglingListFailureStaysVisibleWithoutAbortingDown(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	listErr := errors.New("connection refused")
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{}, listErr)

	// ensureImagesDown itself must not fail: the listing error is only
	// surfaced when the returned op actually runs, same as every other
	// down op, so it can't block ops collected/scheduled around it.
	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.Error(t, opErr, listErr.Error())

	events := make([]string, len(rec.resources))
	for i, e := range rec.resources {
		events[i] = e.ID + ": " + e.Text
	}
	// no "Dangling images: Removing"/"Removed" — only the error is visible.
	assert.DeepEqual(t, events, []string{"Dangling images: " + compose.StatusError})
	assert.Equal(t, rec.resources[0].Status, compose.Error)
	assert.Equal(t, rec.resources[0].Details, listErr.Error())
}

// TestEnsureImagesDown_PartialRemovalFailureStaysVisibleAlongsideRemoved
// guards the other half of the same request: when some dangling images
// fail to be removed for real (not just a "someone else already removed
// it" race), the failure must be visible in addition to — not instead of —
// the "Removed" status for the ones that did succeed, and the op still
// reports an error so the overall `down` failure is not silently swallowed.
func TestEnsureImagesDown_PartialRemovalFailureStaysVisibleAlongsideRemoved(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:ok"},
		{ID: "sha256:in-use"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:ok", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:in-use", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used by a container"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true})
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.ErrorContains(t, opErr, "sha256:in-use")

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Text, "Removed")
	assert.Equal(t, rec.resources[1].Status, compose.Warning)
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "sha256:in-use")
}

// TestDownRemovesRetainedPreStartHookContainers verifies that compose down finds and
// removes pre_start hook containers that were retained after a failed hook run.
// These containers lack ConfigHashLabel so the normal getContainers path never sees them.
func TestDownRemovesRetainedPreStartHookContainers(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	// No regular service containers running.
	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).
		Return(client.ContainerListResult{}, nil)
	api.EXPECT().VolumeList(gomock.Any(), client.VolumeListOptions{
		Filters: projectFilter(strings.ToLower(testProject)),
	}).Return(client.VolumeListResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)),
	}).Return(client.NetworkListResult{}, nil)

	// Hook container scan finds one retained pre_start container.
	hookCtr := container.Summary{
		ID:    "hook-1",
		Names: []string{"/hook-1"},
		Labels: map[string]string{
			compose.ProjectLabel: strings.ToLower(testProject),
			compose.ServiceLabel: "service1",
			compose.HookLabel:    preStartHookType,
		},
	}
	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).
		Return(client.ContainerListResult{Items: []container.Summary{hookCtr}}, nil)

	// The hook container must be force-removed with volumes.
	api.EXPECT().ContainerRemove(gomock.Any(), "hook-1",
		client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{})
	assert.NilError(t, err)
}

// TestDownHookContainerRemovalFailureIsNonFatal verifies that a failure to remove a
// retained hook container is logged as a warning but does not abort compose down.
func TestDownHookContainerRemovalFailureIsNonFatal(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	// No regular service containers running.
	api.EXPECT().ContainerList(gomock.Any(), projectFilterListOpt(false)).
		Return(client.ContainerListResult{}, nil)
	api.EXPECT().VolumeList(gomock.Any(), client.VolumeListOptions{
		Filters: projectFilter(strings.ToLower(testProject)),
	}).Return(client.VolumeListResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)),
	}).Return(client.NetworkListResult{}, nil)

	// Hook scan finds one container.
	hookCtr := container.Summary{
		ID:    "hook-2",
		Names: []string{"/hook-2"},
		Labels: map[string]string{
			compose.ProjectLabel: strings.ToLower(testProject),
			compose.ServiceLabel: "service1",
			compose.HookLabel:    preStartHookType,
		},
	}
	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).
		Return(client.ContainerListResult{Items: []container.Summary{hookCtr}}, nil)

	// Removal fails — Down must still return nil.
	api.EXPECT().ContainerRemove(gomock.Any(), "hook-2",
		client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}).
		Return(client.ContainerRemoveResult{}, errors.New("daemon busy"))

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{})
	assert.NilError(t, err)
}

// A relay stands in for the service on the network but is a shell-less
// scratch binary: pre_stop has no process inside it to act on. No
// ExecCreate expectation is set: per newStartTestService, gomock fails the
// test if the hook still runs.
func TestStopContainerSkipsPreStopForRelay(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	service := types.ServiceConfig{
		Name:    "db",
		PreStop: []types.ServiceHook{{Command: types.ShellCommand{"quiesce"}}},
	}
	relay := serviceContainer("db", 1, container.StateRunning)
	relay.Labels[compose.RelayLabel] = "abc123"

	apiClient.EXPECT().ContainerStop(gomock.Any(), relay.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, nil)

	err := svc.stopContainer(t.Context(), &service, relay, nil, nil)
	assert.NilError(t, err)
}

// runningOneOff builds a RUNNING `compose run` container of the given service.
func runningOneOff(service, id string) container.Summary {
	c := testContainer(service, id, true)
	c.State = container.StateRunning
	return c
}
