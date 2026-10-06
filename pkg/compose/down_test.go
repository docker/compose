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
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

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

// An invalid image prune mode must be rejected before any resource is
// touched: the mocks carry no expectation, so a single daemon call would
// fail the test. Guards the down() precondition — validating mid-down,
// after containers were removed, would leave the teardown half done.
func TestDownRejectsInvalidImagePruneModeUpfront(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	_, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{
		Images: "bogus",
	})
	assert.ErrorContains(t, err, `invalid image prune mode "bogus"`)
}

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

	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	api.EXPECT().ContainerList(gomock.Any(), hookFilterListOpt()).Return(client.ContainerListResult{}, nil)

	err = tested.Down(t.Context(), strings.ToLower(testProject), compose.DownOptions{})
	assert.NilError(t, err)
}

// TestDown_ConcurrencyIsBoundedAcrossServices guards against the same
// per-service budget leak fixed on restart.go and stop.go:
// InReverseDependencyOrder dispatches independent services concurrently, so
// a limiter created fresh inside removeContainers for each service visit
// would let each one spend its own maxConcurrency budget at the same time.
func TestDown_ConcurrencyIsBoundedAcrossServices(t *testing.T) {
	svc, apiClient := newTestService(t, WithMaxConcurrency(1))

	const numServices = 4
	project, containers := nIndependentServiceContainers(numServices)

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: containers}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil) // removePreStartHookContainers lookup

	// no relay-link network for this project (no provider service involved)
	apiClient.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("prj").Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	apiClient.EXPECT().ContainerStop(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(client.ContainerStopResult{}, nil).
		Times(numServices)

	tracker := &peakConcurrencyTracker{}
	apiClient.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
			tracker.leave()
			return client.ContainerRemoveResult{}, nil
		}).
		Times(numServices)

	err := svc.down(t.Context(), "prj", compose.DownOptions{Project: project})
	assert.NilError(t, err)
	assert.Equal(t, tracker.Peak(), 1, "down must never run more than maxConcurrency ContainerRemove calls at once")
}

// TestDown_ImagePruningSharesConcurrencyBudgetAcrossOps guards against a
// regression flagged in review: removeTaggedImagesOp and
// removeDanglingImagesOp each opened their own bounded errgroup sized to the
// full maxConcurrency budget, but run concurrently as two independent down
// ops -- so combined image-removal calls could reach 2x maxConcurrency
// instead of sharing one budget, the same class of leak
// TestDown_ConcurrencyIsBoundedAcrossServices guards for container removal.
func TestDown_ImagePruningSharesConcurrencyBudgetAcrossOps(t *testing.T) {
	svc, apiClient := newTestService(t, WithMaxConcurrency(2))

	project := &types.Project{Name: "prj"}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil) // removePreStartHookContainers lookup

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:tag1", RepoTags: []string{"repo/tag1:latest"}},
		{ID: "sha256:tag2", RepoTags: []string{"repo/tag2:latest"}},
	}}, nil)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:dangling1"},
		{ID: "sha256:dangling2"},
	}}, nil)

	// no relay-link network for this project (no provider service involved)
	apiClient.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("prj").Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	tracker := &peakConcurrencyTracker{}
	apiClient.EXPECT().ImageRemove(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
			tracker.leave()
			return client.ImageRemoveResult{}, nil
		}).
		Times(4)

	err := svc.down(t.Context(), "prj", compose.DownOptions{Project: project, Images: "local", RemoveOrphans: true})
	assert.NilError(t, err)
	assert.Assert(t, tracker.Peak() <= 2, "tagged- and dangling-image removal must share the same budget, got peak %d", tracker.Peak())
}

// TestDown_NetworkAndImageRemovalShareConcurrencyBudget guards against a
// regression flagged in review: the outer ops dispatch bounded only the
// *count* of concurrently-running down ops, independent of the shared
// limiter each op's own engine calls are gated on -- so a single ordinary
// project network plus --rmi could spend up to 2x maxConcurrency, the same
// class of leak TestDown_ImagePruningSharesConcurrencyBudgetAcrossOps guards
// between the two image ops.
func TestDown_NetworkAndImageRemovalShareConcurrencyBudget(t *testing.T) {
	svc, apiClient := newTestService(t, WithMaxConcurrency(2))

	project := &types.Project{
		Name: "prj",
		Networks: types.Networks{
			"default": {Name: "prj_default"},
		},
	}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil) // removePreStartHookContainers lookup

	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:tag1", RepoTags: []string{"repo/tag1:latest"}},
		{ID: "sha256:tag2", RepoTags: []string{"repo/tag2:latest"}},
	}}, nil)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{}, nil)

	tracker := &peakConcurrencyTracker{}
	apiClient.EXPECT().ImageRemove(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond) // widen the window for a concurrency violation to show up
			tracker.leave()
			return client.ImageRemoveResult{}, nil
		}).
		Times(2)

	apiClient.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("prj").Add("label", networkFilter("default")),
	}).Return(client.NetworkListResult{Items: []network.Summary{
		{Network: network.Network{ID: "net1", Name: "prj_default"}},
	}}, nil)
	apiClient.EXPECT().NetworkInspect(gomock.Any(), "net1", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
			tracker.enter()
			time.Sleep(20 * time.Millisecond)
			tracker.leave()
			return client.NetworkInspectResult{Network: network.Inspect{Network: network.Network{ID: "net1"}}}, nil
		})
	apiClient.EXPECT().NetworkRemove(gomock.Any(), "net1", gomock.Any()).
		Return(client.NetworkRemoveResult{}, nil)

	// no relay-link network for this project (no provider service involved)
	apiClient.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("prj").Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	err := svc.down(t.Context(), "prj", compose.DownOptions{Project: project, Images: "local", RemoveOrphans: true})
	assert.NilError(t, err)
	assert.Assert(t, tracker.Peak() <= 2, "network- and image-removal ops must share the same concurrency budget, got peak %d", tracker.Peak())
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

	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

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

	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

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

	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

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

	// no relay-link network for this project (no provider service involved);
	// down() runs twice in this test (--rmi=local then --rmi=all)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil).AnyTimes()

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

	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

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

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	opErr := ops[0]()
	assert.Error(t, opErr, listErr.Error())
	assert.NilError(t, ops[1]()) // dangling-images op is independent, still runs fine

	assert.Equal(t, len(rec.resources), 1)
	assert.Equal(t, rec.resources[0].ID, "Tagged images")
	assert.Equal(t, rec.resources[0].Status, compose.Error)
	assert.Equal(t, rec.resources[0].Details, listErr.Error())
}

// liveCtx matches a context.Context argument only if it hasn't been
// canceled, to catch a shared errgroup ctx getting canceled by a sibling
// goroutine's failure and silently masking later calls that would run fine
// with an unmocked API client (which does check ctx and would fail them
// with "context canceled" instead of ever reaching the daemon).
type liveCtx struct{}

func (liveCtx) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx.Err() == nil
}

func (liveCtx) String() string {
	return "is a non-canceled context"
}

// TestRemoveTaggedImagesOp_ContinuesAfterOneImageFails guards a regression
// caught in review: removeTaggedImagesOp used to share its errgroup's
// derived ctx with every image removal via errgroup.WithContext, so a real
// failure on one image canceled that ctx and made every image still queued
// behind it fail with "context canceled" instead of actually being
// attempted, unlike the independent per-image ops it replaced.
func TestRemoveTaggedImagesOp_ContinuesAfterOneImageFails(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, cli := prepareMocks(mockCtrl)
	rec := &capturingEvents{}
	svcIface, err := NewComposeService(cli, WithEventProcessor(rec), WithMaxConcurrency(1))
	assert.NilError(t, err)
	svc := svcIface.(*composeService)

	project := &types.Project{Name: "prj"}
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "false"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:aaa", RepoTags: []string{"repo/a:latest"}},
		{ID: "sha256:bbb", RepoTags: []string{"repo/b:latest"}},
		{ID: "sha256:ccc", RepoTags: []string{"repo/c:latest"}},
	}}, nil)
	daemonErr := errors.New("i/o timeout")
	apiClient.EXPECT().ImageRemove(gomock.Any(), "repo/a:latest", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, daemonErr)
	apiClient.EXPECT().ImageRemove(liveCtx{}, "repo/b:latest", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(liveCtx{}, "repo/c:latest", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)

	opErr := svc.removeTaggedImagesOp(t.Context(), project, ImagePruneOptions{Mode: ImagePruneLocal, RemoveOrphans: true}, newOptionalLimiter(1))
	assert.ErrorContains(t, opErr, daemonErr.Error())

	// dispatch order across images is no longer deterministic now that the
	// concurrency bound comes from a shared limiter acquired inside each
	// goroutine (see removeImages) rather than errgroup.SetLimit blocking the
	// submitting loop itself -- only the per-image Removing->Removed sequence
	// (enforced by removeResource) still holds, so compare as a set.
	events := make([]string, len(rec.resources))
	for i, e := range rec.resources {
		events[i] = e.ID + ": " + e.Text
	}
	sort.Strings(events)
	assert.DeepEqual(t, events, []string{
		"Image repo/a:latest: Removing",
		"Image repo/b:latest: Removed",
		"Image repo/b:latest: Removing",
		"Image repo/c:latest: Removed",
		"Image repo/c:latest: Removing",
	})
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
	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
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

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local"}, nil)
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

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	for _, op := range ops {
		assert.NilError(t, op())
	}
}

// newDanglingImagesFixture sets up a composeService with a capturing event
// recorder and a bare "prj" project, and stubs the tagged-image listing
// (ImagesToPrune's own query) as empty so the tests below can focus solely
// on the dangling-images op. Shared by the tests below guarding the fix
// for #14219.
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

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
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

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local"}, nil)
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
	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
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
		{ID: "sha256:fails"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:ok", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:fails", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrPermissionDenied.WithMessage("permission denied"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.ErrorContains(t, opErr, "sha256:fails")

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Text, "Removed")
	assert.Equal(t, rec.resources[1].Status, compose.Warning)
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "sha256:fails")
}

// TestEnsureImagesDown_TotalRemovalFailureReportsPlainError guards the fix
// suggested in review: when NO eligible dangling image actually gets
// removed (every one hits a real error), the "Removed" label must not be
// shown at all — claiming "Removed" when nothing was would be dishonest —
// a plain error is reported instead.
func TestEnsureImagesDown_TotalRemovalFailureReportsPlainError(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:fails"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:fails", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrPermissionDenied.WithMessage("permission denied"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.ErrorContains(t, opErr, "sha256:fails")

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Error)
	assert.Equal(t, rec.resources[1].Text, compose.StatusError)
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "sha256:fails")
}

// TestEnsureImagesDown_TotalFailureAlsoMentionsStillInUse guards the mirror
// gap found in review of the merged err!=nil case: when NOTHING was removed
// (a plain error is reported, same as TotalRemovalFailureReportsPlainError)
// but the batch also has a still-in-use image alongside the genuine
// failure, that still-in-use count must not be silently dropped just
// because the removed==0 path reports through errorEvent instead of the
// Warning+"Removed" path.
func TestEnsureImagesDown_TotalFailureAlsoMentionsStillInUse(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:in-use"},
		{ID: "sha256:fails"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:in-use", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used by a container"))
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:fails", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrPermissionDenied.WithMessage("permission denied"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.ErrorContains(t, opErr, "sha256:fails")

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Error)
	assert.Equal(t, rec.resources[1].Text, compose.StatusError)
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "sha256:fails")
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "1 image(s) still in use")
}

// TestEnsureImagesDown_AllEligibleStillInUseDoesNotClaimRemoved guards a
// regression caught by review after the Conflict-tolerance fix landed: when
// every eligible dangling image is tolerated as still in use (or already
// gone), removeImages returns (nil, nil) — no error — so a naive `if err !=
// nil` guard alone would fall through and report a false "Removed" even
// though nothing was. The op must report the terminal state honestly
// instead, without leaving the earlier "Removing" event stuck unresolved.
func TestEnsureImagesDown_AllEligibleStillInUseDoesNotClaimRemoved(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:in-use"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:in-use", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used by a container"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	assert.NilError(t, ops[1]())

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Warning)
	assert.Equal(t, rec.resources[1].Text, "Resource is still in use")
	assert.Equal(t, rec.resources[1].Details, "1 image(s) still in use")
}

// TestEnsureImagesDown_MixedRemovedAndStillInUseKeepsBothVisible guards a
// second regression caught by the same review pass: when the batch is
// mixed — one image actually removed, another still in use, no genuine
// error — the earlier fix's `len(removed) == 0` guard alone can't see the
// still-in-use one either (removed is non-empty), so it would fall through
// to a plain "Removed" and hide that one image never actually went away.
func TestEnsureImagesDown_MixedRemovedAndStillInUseKeepsBothVisible(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:removed"},
		{ID: "sha256:in-use"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:removed", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:in-use", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used by a container"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	assert.NilError(t, ops[1]())

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Warning)
	assert.Equal(t, rec.resources[1].Text, "Removed")
	assert.Equal(t, rec.resources[1].Details, "1 image(s) still in use")
}

// TestEnsureImagesDown_PartialFailureAlsoMentionsStillInUse guards a gap
// found in review of the switch above: when a genuine failure AND a
// still-in-use image both occur in the same batch (with at least one
// image actually removed), the `err != nil` branch fires first and must
// not silently drop the still-in-use count the way a naive "just show
// err.Error()" would — both facts belong in the same message.
func TestEnsureImagesDown_PartialFailureAlsoMentionsStillInUse(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:removed"},
		{ID: "sha256:in-use"},
		{ID: "sha256:fails"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:removed", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:in-use", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrConflict.WithMessage("image is being used by a container"))
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:fails", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrPermissionDenied.WithMessage("permission denied"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	opErr := ops[1]()
	assert.ErrorContains(t, opErr, "sha256:fails")

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Warning)
	assert.Equal(t, rec.resources[1].Text, "Removed")
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "sha256:fails")
	assert.ErrorContains(t, errors.New(rec.resources[1].Details), "1 image(s) still in use")
}

// TestEnsureImagesDown_AllEligibleAlreadyGoneReportsNoResourceFound guards a
// third combination the same review pass flagged: when every eligible
// image turns out to already be gone (a benign race, not a conflict), the
// aggregate must say so accurately — not "Resource is still in use", which
// would be factually wrong for an image that no longer exists at all.
func TestEnsureImagesDown_AllEligibleAlreadyGoneReportsNoResourceFound(t *testing.T) {
	apiClient, svc, rec, project := newDanglingImagesFixture(t)
	apiClient.EXPECT().ImageList(gomock.Any(), client.ImageListOptions{
		Filters: projectFilter("prj").Add("dangling", "true"),
	}).Return(client.ImageListResult{Items: []image.Summary{
		{ID: "sha256:already-gone"},
	}}, nil)
	apiClient.EXPECT().ImageRemove(gomock.Any(), "sha256:already-gone", client.ImageRemoveOptions{}).
		Return(client.ImageRemoveResult{}, errdefs.ErrNotFound.WithMessage("already removed"))

	ops := svc.ensureImagesDown(t.Context(), project, compose.DownOptions{Images: "local", RemoveOrphans: true}, nil)
	assert.Equal(t, len(ops), 2) // tagged-images op + dangling-images op

	assert.NilError(t, ops[0]()) // tagged-images op: nothing to prune, trivially succeeds
	assert.NilError(t, ops[1]())

	assert.Equal(t, len(rec.resources), 2)
	assert.Equal(t, rec.resources[0].ID, "Dangling images")
	assert.Equal(t, rec.resources[0].Text, "Removing")
	assert.Equal(t, rec.resources[1].ID, "Dangling images")
	assert.Equal(t, rec.resources[1].Status, compose.Done)
	assert.Equal(t, rec.resources[1].Text, "Warning: No resource found to remove")
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
	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
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
	// no relay-link network for this project (no provider service involved)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", compose.RelayNetworkLabel),
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
