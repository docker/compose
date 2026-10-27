//go:build !windows

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

package compose

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// callsOf counts the recorded engine calls named call.
func callsOf(d *interactiveUpDaemon, call string) int {
	n := 0
	for _, c := range d.recorded() {
		if c == call {
			n++
		}
	}
	return n
}

// assertCreatedThenStarted checks that the daemon was asked to create the
// container, then to start it.
func assertCreatedThenStarted(t *testing.T, d *interactiveUpDaemon) {
	t.Helper()
	create, start := d.indexOf("create"), d.indexOf("start")
	assert.Assert(t, create >= 0, "the container was never created: %v", d.recorded())
	assert.Assert(t, start >= 0, "the container was never started: %v", d.recorded())
	assert.Assert(t, create < start, "the container must be created before it is started: %v", d.recorded())
}

// TestUpDetachedCreatesThenStartsTheContainers pins that a detached up leaves
// the application running: its plan covers both phases, so that what it creates
// it starts, with no session attached to it.
func TestUpDetachedCreatesThenStartsTheContainers(t *testing.T) {
	h := newInteractiveUpHarness(t)

	assert.NilError(t, h.svc.Up(t.Context(), interactiveUpProject(), api.UpOptions{}))

	assertCreatedThenStarted(t, h.daemon)
	assert.Equal(t, h.daemon.indexOf("attach"), -1, "a detached up attaches to nothing: %v", h.daemon.recorded())
}

// TestScaleCreatesThenStartsTheReplicas pins that scale starts the replicas it
// creates: it converges the project and starts it in a single plan, as a
// detached up does, and so does the rebuild of a service by watch, which goes
// through the same path.
func TestScaleCreatesThenStartsTheReplicas(t *testing.T) {
	h := newInteractiveUpHarness(t)

	err := h.svc.Scale(t.Context(), interactiveUpProject(), api.ScaleOptions{Replicas: map[string]int{"web": 1}})

	assert.NilError(t, err)
	assertCreatedThenStarted(t, h.daemon)
}

// TestRunStartsTheDependenciesOfTheOneOff pins that `compose run` brings up
// the dependencies of the service it runs -- created, then started -- and
// only those: the service itself is the one-off container's business.
func TestRunStartsTheDependenciesOfTheOneOff(t *testing.T) {
	h := newInteractiveUpHarness(t)
	project := interactiveUpProject()
	app := types.ServiceConfig{Name: "app", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	app.DependsOn = types.DependsOnConfig{"web": {Condition: types.ServiceConditionStarted, Required: true}}
	project.Services["app"] = app

	err := h.svc.startDependencies(t.Context(), project, api.RunOptions{Project: project, Service: "app"})

	assert.NilError(t, err)
	assertCreatedThenStarted(t, h.daemon)
	assert.Equal(t, callsOf(h.daemon, "create"), 1, "only the dependency is created, not the service run: %v", h.daemon.recorded())
	assert.Equal(t, callsOf(h.daemon, "start"), 1, "only the dependency is started: %v", h.daemon.recorded())
}

// TestUpDetachedWaitVerifiesTheApplicationIsReady pins what --wait adds to a
// detached up: once the containers are started, it checks that they are up --
// an exited container fails the up, naming it, and one that does not become
// healthy in time fails it with the --wait-timeout that elapsed -- where
// without --wait the up returns as soon as the containers are started.
func TestUpDetachedWaitVerifiesTheApplicationIsReady(t *testing.T) {
	healthcheck := &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	projectWithHealthcheck := func() *types.Project {
		project := interactiveUpProject()
		web := project.Services["web"]
		web.HealthCheck = healthcheck
		project.Services["web"] = web
		return project
	}

	t.Run("a container that exited fails the up", func(t *testing.T) {
		h := newInteractiveUpHarness(t)

		err := h.svc.Up(t.Context(), interactiveUpProject(), api.UpOptions{Start: api.StartOptions{Wait: true, WaitTimeout: 5 * time.Second}})

		assert.ErrorContains(t, err, "container test-web-1 exited")
	})

	t.Run("a container that stays unhealthy fails the up at the --wait-timeout", func(t *testing.T) {
		h := newInteractiveUpHarness(t)
		h.daemon.inspectState = &container.State{Status: container.StateRunning, Health: &container.Health{Status: container.Starting}}

		err := h.svc.Up(t.Context(), projectWithHealthcheck(), api.UpOptions{Start: api.StartOptions{Wait: true, WaitTimeout: 100 * time.Millisecond}})

		assert.Error(t, err, "application not healthy after 100ms")
	})

	t.Run("without --wait nothing is verified", func(t *testing.T) {
		h := newInteractiveUpHarness(t)

		assert.NilError(t, h.svc.Up(t.Context(), interactiveUpProject(), api.UpOptions{}))
	})
}

// TestWatchRebuildTouchesTheRebuiltServiceAndItsDependentsOnly pins which of
// the project's containers a rebuild by watch converges: those of the rebuilt
// service and of the services depending on it (their configuration may
// diverge from what the rebuilt service now is), and no others -- neither its
// dependencies, which are not rebuilt either, nor the services unrelated to
// it, whose stopped containers have no business being started by a rebuild.
// The images are not built here (no service has a build section); the project
// the rebuild works on is what is narrowed.
func TestWatchRebuildTouchesTheRebuiltServiceAndItsDependentsOnly(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)
	service := func(name string, dependsOn ...string) types.ServiceConfig {
		s := types.ServiceConfig{Name: name, ContainerSpec: types.ContainerSpec{Image: "alpine"}}
		if len(dependsOn) > 0 {
			s.DependsOn = types.DependsOnConfig{}
		}
		for _, dep := range dependsOn {
			s.DependsOn[dep] = types.ServiceDependency{Condition: types.ServiceConditionStarted, Required: true}
		}
		return s
	}
	project := &types.Project{
		Name: "prj",
		Services: types.Services{
			"app":       service("app", "database"),
			"proxy":     service("proxy", "app"),
			"database":  service("database"),
			"unrelated": service("unrelated"),
		},
		Networks: types.Networks{},
		Volumes:  types.Volumes{},
	}
	existing := []container.Summary{
		stoppedServiceContainer("proxy", 1, container.StateExited),
		stoppedServiceContainer("database", 1, container.StateExited),
		stoppedServiceContainer("unrelated", 1, container.StateExited),
	}

	// what the daemon is asked to do, by container ID
	var mu sync.Mutex
	touched := map[string]bool{}
	touch := func(id string) {
		mu.Lock()
		defer mu.Unlock()
		touched[id] = true
	}
	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine", gomock.Any()).AnyTimes().
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:alpine"}}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.VolumeListResult{}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
		Return(client.ContainerListResult{Items: existing}, nil)
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			touch("new:" + opts.Name)
			return client.ContainerCreateResult{ID: "new-" + opts.Name}, nil
		})
	apiClient.EXPECT().ContainerInspect(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			return client.ContainerInspectResult{Container: container.InspectResponse{
				ID: id, Name: "/" + strings.TrimPrefix(id, "new-"), Config: &container.Config{}, NetworkSettings: &container.NetworkSettings{},
			}}, nil
		})
	apiClient.EXPECT().ContainerStop(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, id string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
			touch(id)
			return client.ContainerStopResult{}, nil
		})
	apiClient.EXPECT().ContainerRemove(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, id string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
			touch(id)
			return client.ContainerRemoveResult{}, nil
		})
	apiClient.EXPECT().ContainerRename(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		Return(client.ContainerRenameResult{}, nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, id string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
			touch(id)
			return client.ContainerStartResult{}, nil
		})

	options := api.WatchOptions{Build: &api.BuildOptions{}, LogTo: &testLogConsumer{}}
	assert.NilError(t, svc.rebuild(t.Context(), project, []string{"app"}, options))

	touchesService := func(name string) bool {
		for id := range touched {
			if strings.Contains(id, "prj-"+name+"-1") {
				return true
			}
		}
		return false
	}
	assert.Assert(t, touchesService("app"), "the rebuilt service is created and started: %v", touched)
	assert.Assert(t, touchesService("proxy"), "the container of the service depending on it is converged: %v", touched)
	assert.Assert(t, !touchesService("database"), "a dependency of the rebuilt service is left alone: %v", touched)
	assert.Assert(t, !touchesService("unrelated"), "a service unrelated to it is left alone: %v", touched)
}
