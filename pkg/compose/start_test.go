//go:build !windows

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
	"archive/tar"
	"context"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// recordingEventProcessor captures progress events for sequence assertions.
// Shared by tests elsewhere in the package that exercise a single container
// operation in isolation (e.g. stopContainer's relay guard in down_test.go).
type recordingEventProcessor struct {
	mu     sync.Mutex
	events []api.Resource
}

func (r *recordingEventProcessor) Start(context.Context, string) {}

func (r *recordingEventProcessor) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

func (r *recordingEventProcessor) Done(string, bool) {}

// summary renders recorded events as "ID: Text" strings.
func (r *recordingEventProcessor) summary() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.ID+": "+e.Text)
	}
	return out
}

func (r *recordingEventProcessor) contains(entry string) bool {
	for _, e := range r.summary() {
		if e == entry {
			return true
		}
	}
	return false
}

// newStartTestService builds a composeService on gomock with a recording
// event processor. Any API call without a matching expectation fails the test,
// so "no expectation" doubles as a "no daemon interaction" assertion.
func newStartTestService(t *testing.T) (*composeService, *mocks.MockAPIClient, *recordingEventProcessor) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()
	// the generic pre_start inheritance goes through getCreateConfigs, which
	// reads the CLI configuration and the daemon host
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("unix:///var/run/docker.sock").AnyTimes()

	rec := &recordingEventProcessor{}
	svc, err := NewComposeService(cli, WithEventProcessor(rec))
	assert.NilError(t, err)
	return svc.(*composeService), apiClient, rec
}

// serviceContainer builds a running container summary the way the daemon
// reports a project's first replica: canonical name, service and
// container-number labels.
func serviceContainer(service string) container.Summary {
	name := "prj-" + service + "-1"
	return container.Summary{
		ID:    name + "-id",
		Names: []string{"/" + name},
		State: container.StateRunning,
		Labels: map[string]string{
			api.ServiceLabel:         service,
			api.ContainerNumberLabel: "1",
		},
	}
}

// stoppedServiceContainer builds a container summary of the given state for
// replica num of service, carrying the labels collectObservedState reads.
func stoppedServiceContainer(service string, num int, state container.ContainerState) container.Summary {
	ctr := serviceContainer(service)
	name := "prj-" + service + "-" + strconv.Itoa(num)
	ctr.ID = name + "-id"
	ctr.Names = []string{"/" + name}
	ctr.State = state
	ctr.Labels[api.ProjectLabel] = "prj"
	ctr.Labels[api.ContainerNumberLabel] = strconv.Itoa(num)
	return ctr
}

// TestStart_NoOpStartIsSilentAndOnlyListsContainers pins how start() wires
// its plan: the observation is scoped to Start (only the container listing
// reaches the daemon -- gomock fails on any network or volume listing), the
// plan is start-only, and a no-op start (everything already running) starts
// nothing and emits no event, not even the synthetic "Running" ones
// up/create report.
func TestStart_NoOpStartIsSilentAndOnlyListsContainers(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)

	web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	project := &types.Project{Name: "prj", Services: types.Services{"web": web}}

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{
			stoppedServiceContainer("web", 1, container.StateRunning),
		}}, nil)

	err := svc.start(t.Context(), "prj", api.StartOptions{Project: project})
	assert.NilError(t, err)
	assert.Equal(t, len(rec.summary()), 0, "a no-op start must be silent: %v", rec.summary())
}

// TestStart_WaitTimeoutBoundsDependencyWaits pins that start() hands
// StartOptions.WaitTimeout to the executor: --wait-timeout bounds a
// depends_on wait even without --wait, and expiring reports the dependency
// timeout while the dependent is never started.
func TestStart_WaitTimeoutBoundsDependencyWaits(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	db := types.ServiceConfig{Name: "db", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	db.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	web.DependsOn = types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}}
	project := &types.Project{Name: "prj", Services: types.Services{"db": db, "web": web}}

	dbCtr := stoppedServiceContainer("db", 1, container.StateRunning)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{
			dbCtr,
			stoppedServiceContainer("web", 1, container.StateExited),
		}}, nil)
	// db never becomes healthy; no ContainerStart expectation for web
	apiClient.EXPECT().ContainerInspect(gomock.Any(), dbCtr.ID, gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:   dbCtr.ID,
			Name: "/prj-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Starting},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		}}, nil).AnyTimes()

	// the outer deadline only turns a missing wait-timeout into a prompt
	// failure instead of a hang
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := svc.start(ctx, "prj", api.StartOptions{Project: project, WaitTimeout: 200 * time.Millisecond})
	assert.Error(t, err, "timeout waiting for dependencies")
}

// TestStart_UseAPISocketServiceGetsItsConfigFile pins that start() applies
// use_api_socket to the project it works on: the config file the option adds is
// injected when a container starts, from that project, so a container created
// beforehand and only started here (`create` then `start`) would otherwise get
// DOCKER_CONFIG pointing at a file that was never created. The caller's project
// is left as it was, whether or not it has the maps the option writes into: the
// project `compose start` loads has no service Environment, a programmatic one
// no Configs.
func TestStart_UseAPISocketServiceGetsItsConfigFile(t *testing.T) {
	tests := []struct {
		name    string
		env     types.MappingWithEquals
		configs types.Configs
	}{
		{name: "project without environment nor configs maps"},
		{name: "project with empty maps", env: types.MappingWithEquals{}, configs: types.Configs{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, apiClient, _ := newStartTestService(t)
			info := &countingContextInfo{}
			svc.contextInfo = info

			web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine", UseAPISocket: true, Environment: tt.env}}
			project := &types.Project{Name: "prj", Services: types.Services{"web": web}, Configs: tt.configs}

			ctr := serviceContainer("web")
			ctr.State = container.StateCreated
			ctr.Labels[api.ProjectLabel] = "prj"
			apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
				Return(client.ContainerListResult{Items: []container.Summary{ctr}}, nil)

			var injected []string
			copied := apiClient.EXPECT().CopyToContainer(gomock.Any(), ctr.ID, gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, opts client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
					tr := tar.NewReader(opts.Content)
					for {
						header, err := tr.Next()
						if err == io.EOF {
							break
						}
						assert.NilError(t, err)
						injected = append(injected, header.Name)
					}
					return client.CopyToContainerResult{}, nil
				})
			apiClient.EXPECT().ContainerStart(gomock.Any(), ctr.ID, gomock.Any()).
				Return(client.ContainerStartResult{}, nil).After(copied)

			err := svc.start(t.Context(), "prj", api.StartOptions{Project: project})

			assert.NilError(t, err)
			assert.DeepEqual(t, injected, []string{"/run/secrets/docker/config.json"})
			assert.Assert(t, info.osTypeCalls.Load() > 0, "use_api_socket must have been applied")
			assert.Equal(t, len(project.Configs), 0, "the caller's configs must not be written to")
			assert.Equal(t, len(project.Services["web"].Environment), 0, "the caller's service environment must not be written to")
		})
	}
}

// countingContextInfo is a Linux engine's context that counts how often
// useAPISocket got past its early return, which tells that it was applied.
type countingContextInfo struct{ osTypeCalls atomic.Int32 }

func (*countingContextInfo) CurrentContext() string { return "default" }
func (c *countingContextInfo) ServerOSType() string {
	c.osTypeCalls.Add(1)
	return "linux"
}
func (*countingContextInfo) BuildKitEnabled() (bool, error) { return false, nil }

// TestStart_AProjectAlreadyRewrittenIsInjectedOnce pins that start() does not
// rewrite twice a project that already went through use_api_socket, as the one
// `run` hands to the dependencies it starts: the config file would be injected
// once per duplicated entry.
func TestStart_AProjectAlreadyRewrittenIsInjectedOnce(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	svc.contextInfo = &countingContextInfo{}

	web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine", UseAPISocket: true, Environment: types.MappingWithEquals{}}}
	project := &types.Project{Name: "prj", Services: types.Services{"web": web}, Configs: types.Configs{}}
	project, err := svc.useAPISocket(project)
	assert.NilError(t, err)

	ctr := serviceContainer("web")
	ctr.State = container.StateCreated
	ctr.Labels[api.ProjectLabel] = "prj"
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{ctr}}, nil)
	copied := apiClient.EXPECT().CopyToContainer(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.CopyToContainerResult{}, nil).Times(1)
	apiClient.EXPECT().ContainerStart(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(copied)

	assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{Project: project}))
	assert.Equal(t, len(project.Services["web"].Configs), 1, "the project handed to start was rewritten again")
}

// TestGetDependencyCondition locks the --wait condition selection: a service
// that others depend on with service_completed_successfully is waited on with
// that condition (or --wait would hang on one-shot services), anything else
// with running_or_healthy.
func TestGetDependencyCondition(t *testing.T) {
	oneShot := types.ServiceConfig{Name: "migrate"}
	web := types.ServiceConfig{
		Name: "web", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
			"migrate": {Condition: types.ServiceConditionCompletedSuccessfully},
		}},
	}
	project := &types.Project{
		Name:     "prj",
		Services: types.Services{"web": web, "migrate": oneShot},
	}

	assert.Equal(t, getDependencyCondition(oneShot, project), types.ServiceConditionCompletedSuccessfully)
	assert.Equal(t, getDependencyCondition(web, project), ServiceConditionRunningOrHealthy)
}

func inspectResultOf(id, name string, status container.ContainerState, exitCode int, health *container.Health) client.ContainerInspectResult {
	config := &container.Config{}
	if health != nil {
		config.Healthcheck = &container.HealthConfig{Test: []string{"CMD", "true"}}
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{
		ID:     id,
		Name:   "/" + name,
		Config: config,
		State:  &container.State{Status: status, ExitCode: exitCode, Health: health},
	}}
}

// TestStart_WaitVerifiesTheApplicationIsReady pins what --wait adds to start:
// once the containers are started, every service has to be up (running, and
// healthy when it has a healthcheck) before start returns, or start fails
// naming the --wait-timeout that elapsed. Without --wait, start returns as soon
// as the containers are started, and checks nothing.
func TestStart_WaitVerifiesTheApplicationIsReady(t *testing.T) {
	web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	web.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	project := &types.Project{Name: "prj", Services: types.Services{"web": web}}
	webCtr := stoppedServiceContainer("web", 1, container.StateRunning)

	newService := func(t *testing.T, health container.HealthStatus) *composeService {
		svc, apiClient, _ := newStartTestService(t)
		apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
			Return(client.ContainerListResult{Items: []container.Summary{webCtr}}, nil).AnyTimes()
		apiClient.EXPECT().ContainerInspect(gomock.Any(), webCtr.ID, gomock.Any()).AnyTimes().
			Return(inspectResultOf(webCtr.ID, "prj-web-1", container.StateRunning, 0, &container.Health{Status: health}), nil)
		return svc
	}

	t.Run("a service not ready in time fails the start", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newService(t, container.Starting)

			err := svc.start(t.Context(), "prj", api.StartOptions{Project: project, Wait: true, WaitTimeout: 2 * time.Second})

			assert.Error(t, err, "application not healthy after 2s")
		})
	})

	t.Run("a service that is ready lets it succeed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newService(t, container.Healthy)

			assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{Project: project, Wait: true, WaitTimeout: 5 * time.Second}))
		})
	})

	t.Run("without --wait nothing is verified", func(t *testing.T) {
		svc := newService(t, container.Starting)

		assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{Project: project}))
	})
}

// TestStart_WaitSatisfiesAOneShotServiceByItsCompletion pins that the --wait
// verification expects a service that others depend on with
// service_completed_successfully to have exited with 0, not to be running: a
// one-shot service is done once it has run, and would otherwise make --wait
// fail forever. One that completes with an error code fails it, naming the
// service and the code.
func TestStart_WaitSatisfiesAOneShotServiceByItsCompletion(t *testing.T) {
	for _, tt := range []struct {
		name     string
		exitCode int
		err      string
	}{
		{name: "the one-shot service completed", exitCode: 0},
		{name: "the one-shot service failed", exitCode: 3, err: `service "init" didn't complete successfully: exit 3`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, apiClient, _ := newStartTestService(t)

				initService := types.ServiceConfig{Name: "init", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
				app := types.ServiceConfig{Name: "app", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
				app.DependsOn = types.DependsOnConfig{"init": {Condition: types.ServiceConditionCompletedSuccessfully, Required: true}}
				project := &types.Project{Name: "prj", Services: types.Services{"init": initService, "app": app}}

				initCtr := stoppedServiceContainer("init", 1, container.StateExited)
				appCtr := stoppedServiceContainer("app", 1, container.StateRunning)
				apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
					Return(client.ContainerListResult{Items: []container.Summary{initCtr, appCtr}}, nil)
				apiClient.EXPECT().ContainerStart(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(client.ContainerStartResult{}, nil)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), initCtr.ID, gomock.Any()).AnyTimes().
					Return(inspectResultOf(initCtr.ID, "prj-init-1", container.StateExited, tt.exitCode, nil), nil)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), appCtr.ID, gomock.Any()).AnyTimes().
					Return(inspectResultOf(appCtr.ID, "prj-app-1", container.StateRunning, 0, nil), nil)

				err := svc.start(t.Context(), "prj", api.StartOptions{Project: project, Wait: true, WaitTimeout: 5 * time.Second})

				if tt.err == "" {
					assert.NilError(t, err)
				} else {
					assert.ErrorContains(t, err, tt.err)
				}
			})
		})
	}
}

// TestStart_StartsTheReplicasOneAfterTheOther pins the order a service's
// stopped replicas start in: by replica number, each start waiting for the
// previous one's.
func TestStart_StartsTheReplicasOneAfterTheOther(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	web := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}, Scale: intPtr(3)}
	project := &types.Project{Name: "prj", Services: types.Services{"web": web}}
	replicas := []container.Summary{
		stoppedServiceContainer("web", 3, container.StateExited),
		stoppedServiceContainer("web", 1, container.StateExited),
		stoppedServiceContainer("web", 2, container.StateExited),
	}
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
		Return(client.ContainerListResult{Items: replicas}, nil)
	first := apiClient.EXPECT().ContainerStart(gomock.Any(), "prj-web-1-id", gomock.Any()).Return(client.ContainerStartResult{}, nil)
	second := apiClient.EXPECT().ContainerStart(gomock.Any(), "prj-web-2-id", gomock.Any()).Return(client.ContainerStartResult{}, nil).After(first)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "prj-web-3-id", gomock.Any()).Return(client.ContainerStartResult{}, nil).After(second)

	assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{Project: project}))
}

// TestStart_WithoutAProjectStartsTheSelectedServicesOnly pins `compose start
// web` run without a compose file: the project is rebuilt from the project's
// containers, and only the services asked for (with what they depend on) are
// started, the other containers of the project staying as they are.
func TestStart_WithoutAProjectStartsTheSelectedServicesOnly(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	webCtr := stoppedServiceContainer("web", 1, container.StateExited)
	jobCtr := stoppedServiceContainer("job", 1, container.StateExited)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
		Return(client.ContainerListResult{Items: []container.Summary{webCtr, jobCtr}}, nil)
	// no expectation to start the job's container: it would fail the test
	apiClient.EXPECT().ContainerStart(gomock.Any(), webCtr.ID, gomock.Any()).Return(client.ContainerStartResult{}, nil)

	assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{AttachTo: []string{"web"}}))
}

// TestStart_WithoutAProjectListsTheContainersWithoutTheOneOffs pins that the
// project `compose start` rebuilds from the containers it finds is made of the
// project's own containers only, never of the one-off ones (the ones `compose
// run` left behind): the listing it asks the daemon for leaves them out.
func TestStart_WithoutAProjectListsTheContainersWithoutTheOneOffs(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	webCtr := stoppedServiceContainer("web", 1, container.StateExited)
	var firstListing client.ContainerListOptions
	var listings atomic.Int32
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
			if listings.Add(1) == 1 {
				firstListing = opts
			}
			return client.ContainerListResult{Items: []container.Summary{webCtr}}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), webCtr.ID, gomock.Any()).Return(client.ContainerStartResult{}, nil)

	assert.NilError(t, svc.start(t.Context(), "prj", api.StartOptions{}))

	assert.Assert(t, firstListing.Filters["label"][oneOffFilter(false)], "the listing must exclude one-off containers: %v", firstListing.Filters)
}
