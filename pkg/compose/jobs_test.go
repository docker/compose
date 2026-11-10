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
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gotest.tools/v3/assert"

	jobsv0 "github.com/docker/compose/v5/internal/jobsapi"
	"github.com/docker/compose/v5/pkg/api"
)

func TestJobTrigger(t *testing.T) {
	yes, no := true, false

	t.Run("no triggers declared is refused", func(t *testing.T) {
		_, err := jobTrigger(types.JobConfig{Name: "migrate"})
		assert.Error(t, err, `job "migrate" has no trigger`)
	})

	t.Run("manual:true translates to a Manual trigger", func(t *testing.T) {
		trigger, err := jobTrigger(types.JobConfig{Name: "migrate", Triggers: &types.TriggerConfig{Manual: &yes}})
		assert.NilError(t, err)
		assert.DeepEqual(t, trigger, &jobsv0.Trigger{Manual: true})
	})

	t.Run("manual:false alone (no schedule) is refused: it has no trigger left", func(t *testing.T) {
		_, err := jobTrigger(types.JobConfig{Name: "migrate", Triggers: &types.TriggerConfig{Manual: &no}})
		assert.Error(t, err, `job "migrate" has no trigger`)
	})

	t.Run("a single schedule translates to a Schedule trigger", func(t *testing.T) {
		trigger, err := jobTrigger(types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *", Timezone: "UTC"}},
		}})
		assert.NilError(t, err)
		assert.DeepEqual(t, trigger, &jobsv0.Trigger{Schedule: &jobsv0.ScheduleTrigger{Cron: "0 3 * * *", Timezone: "UTC"}})
	})

	t.Run("more than one schedule is refused", func(t *testing.T) {
		_, err := jobTrigger(types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}, {Cron: "0 4 * * *"}},
		}})
		assert.Error(t, err, `job "backup" declares 2 schedules, exactly one is supported`)
	})

	// registerScheduledJobs selects a job on HasSchedule alone, so a job also
	// declaring manual:true must be an error here, not a Manual-only Trigger
	// that drops the cron silently.
	t.Run("manual:true together with a schedule is refused, not silently resolved to Manual", func(t *testing.T) {
		_, err := jobTrigger(types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Manual:   &yes,
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}})
		assert.Error(t, err, `job "backup" declares both manual:true and a schedule, exactly one is supported`)
	})

	t.Run("manual:false together with a schedule keeps the schedule", func(t *testing.T) {
		trigger, err := jobTrigger(types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Manual:   &no,
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}})
		assert.NilError(t, err)
		assert.DeepEqual(t, trigger, &jobsv0.Trigger{Schedule: &jobsv0.ScheduleTrigger{Cron: "0 3 * * *"}})
	})
}

func TestHasSchedule(t *testing.T) {
	assert.Assert(t, !HasSchedule(types.JobConfig{}))
	assert.Assert(t, !HasSchedule(types.JobConfig{Triggers: &types.TriggerConfig{}}))
	assert.Assert(t, HasSchedule(types.JobConfig{Triggers: &types.TriggerConfig{
		Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
	}}))
}

func TestManualTriggerDisabled(t *testing.T) {
	yes, no := true, false

	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{}), "no triggers at all is not an opt-out")
	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{}}), "an unset Manual is not an opt-out")
	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{Manual: &yes}}))
	assert.Assert(t, ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{Manual: &no}}))
}

func TestSortedJobNames(t *testing.T) {
	jobs := types.Jobs{
		"backup":  {Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}}},
		"migrate": {},
		"report":  {},
	}

	assert.DeepEqual(t, sortedJobNames(jobs, nil), []string{"backup", "migrate", "report"})
	assert.DeepEqual(t, sortedJobNames(jobs, HasSchedule), []string{"backup"})
	assert.DeepEqual(t, sortedJobNames(types.Jobs{}, HasSchedule), []string(nil))
}

func TestJobChangedErr(t *testing.T) {
	assert.Error(t, jobChangedErr("backup", "up"),
		`job "backup" has changed: run `+"`docker compose down`"+` to remove it, then `+"`up`"+` again`)
	assert.Error(t, jobChangedErr("migrate", "run"),
		`job "migrate" has changed: run `+"`docker compose down`"+` to remove it, then `+"`run`"+` again`)
}

func TestManualTriggerDisabledErr(t *testing.T) {
	assert.Error(t, ManualTriggerDisabledErr("rotation"),
		`job "rotation" is declared with manual: false, it cannot be run manually`)
}

// scopedProjectForJob is what lets registerScheduledJobs run a scheduled
// job through the same useAPISocket/ensureImagesExists/ensureModels passes
// a manually-run job gets for free from materializeManualJob — without the
// job ever joining the real project.Services the reconciliation loop
// iterates over.
func TestScopedProjectForJob(t *testing.T) {
	project := &types.Project{
		Name:             "myproject",
		DisabledServices: types.Services{"off": {Name: "off"}},
		Services: types.Services{
			"db": {Name: "db"},
		},
		Jobs: types.Jobs{
			"backup": {
				Name:       "backup",
				Extensions: types.Extensions{"x-team": "platform"},
				ContainerSpec: types.ContainerSpec{
					Image:       "backup-tool",
					Environment: types.MappingWithEquals{"BAR": strPtr("original")},
					Volumes:     []types.ServiceVolumeConfig{{Type: types.VolumeTypeImage, Source: "original", Target: "/data"}},
					Configs:     []types.ServiceConfigObjConfig{{Source: "original"}},
				},
			},
		},
		Configs: types.Configs{
			"cfg": {Content: "original"},
		},
	}
	job := project.Jobs["backup"]

	scoped := scopedProjectForJob(project, "backup", job)

	t.Run("the job is materialized into the scoped copy's Services", func(t *testing.T) {
		svc, err := scoped.GetService("backup")
		assert.NilError(t, err)
		assert.Equal(t, svc.Image, "backup-tool")
		assert.Equal(t, svc.Extensions["x-team"], "platform")
	})

	// the passes run on the scoped copy (pull, models, credentials) must see the
	// job alone: the services are `up`'s, which already went through them
	t.Run("the job is the only service, the others are disabled", func(t *testing.T) {
		assert.DeepEqual(t, scoped.ServiceNames(), []string{"backup"})
		assert.DeepEqual(t, scoped.DisabledServiceNames(), []string{"db", "off"})
	})

	t.Run("the real project is never mutated", func(t *testing.T) {
		_, ok := project.Services["backup"]
		assert.Assert(t, !ok, "the job must not leak into the shared project's Services")
		assert.Equal(t, len(project.Services), 1)
		assert.Equal(t, len(project.DisabledServices), 1)
	})

	t.Run("Configs is its own map, not shared with the real project", func(t *testing.T) {
		scoped.Configs["new"] = types.ConfigObjConfig{Content: "added"}
		_, ok := project.Configs["new"]
		assert.Assert(t, !ok, "writing to the scoped copy's Configs must not be visible on the shared project — registerScheduledJobs runs this concurrently per job")
		assert.Equal(t, project.Configs["cfg"].Content, "original")
	})

	t.Run("the job's own Environment, Volumes and Configs are not shared with project.Jobs", func(t *testing.T) {
		svc, err := scoped.GetService("backup")
		assert.NilError(t, err)
		// useAPISocket, ensureModels and ensureImagesExists mutate these in
		// place on the scoped job service: project.Jobs must stay untouched.
		svc.Environment["DOCKER_CONFIG"] = strPtr("added")
		svc.Volumes[0].Source = "mutated"
		svc.Configs[0].Source = "mutated"
		original := project.Jobs["backup"]
		_, ok := original.Environment["DOCKER_CONFIG"]
		assert.Assert(t, !ok, "writing to the scoped job's Environment must not be visible on project.Jobs")
		assert.Equal(t, original.Volumes[0].Source, "original")
		assert.Equal(t, original.Configs[0].Source, "original")
	})
}

// fakeJobsClient is a minimal jobsv0.Jobs double for createJobRun's routing:
// only Create/Run/CreateAndRun are ever exercised by RunJob, so every other
// method is left to the embedded nil interface — calling one would panic,
// which is exactly the loud failure a test relying on it deserves.
type fakeJobsClient struct {
	jobsv0.Jobs

	mu                sync.Mutex // registerScheduledJobs calls Create concurrently
	createCalls       []*jobsv0.CreateRequest
	createErr         error
	createAndRunCalls []*jobsv0.CreateAndRunRequest
	createAndRunReply *jobsv0.CreateAndRunReply
	createAndRunErr   error
	runCalls          []*jobsv0.RunRequest
	runReply          *jobsv0.RunReply
	runErr            error
	listCalls         []*jobsv0.ListRequest
	listReply         *jobsv0.ListReply
	listErr           error
	waitCalls         []*jobsv0.WaitRequest
	waitReply         *jobsv0.WaitReply
	waitErr           error
	removeCalls       []*jobsv0.RemoveRequest
	cancelCalls       []*jobsv0.CancelRequest
	cancelErr         error
	removeErr         error
}

func (f *fakeJobsClient) Create(_ context.Context, req *jobsv0.CreateRequest) (*jobsv0.CreateReply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &jobsv0.CreateReply{Job: &jobsv0.Job{Name: req.Name}}, nil
}

func (f *fakeJobsClient) CreateAndRun(_ context.Context, req *jobsv0.CreateAndRunRequest) (*jobsv0.CreateAndRunReply, error) {
	f.createAndRunCalls = append(f.createAndRunCalls, req)
	return f.createAndRunReply, f.createAndRunErr
}

func (f *fakeJobsClient) Run(_ context.Context, req *jobsv0.RunRequest) (*jobsv0.RunReply, error) {
	f.runCalls = append(f.runCalls, req)
	return f.runReply, f.runErr
}

func (f *fakeJobsClient) Remove(_ context.Context, req *jobsv0.RemoveRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls = append(f.removeCalls, req)
	return f.removeErr
}

func (f *fakeJobsClient) Cancel(_ context.Context, req *jobsv0.CancelRequest) (*jobsv0.CancelReply, error) {
	f.cancelCalls = append(f.cancelCalls, req)
	return &jobsv0.CancelReply{}, f.cancelErr
}

func (f *fakeJobsClient) Wait(_ context.Context, req *jobsv0.WaitRequest) (*jobsv0.WaitReply, error) {
	f.waitCalls = append(f.waitCalls, req)
	return f.waitReply, f.waitErr
}

func (f *fakeJobsClient) List(_ context.Context, req *jobsv0.ListRequest) (*jobsv0.ListReply, error) {
	f.listCalls = append(f.listCalls, req)
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listReply == nil {
		return &jobsv0.ListReply{}, nil
	}
	return f.listReply, nil
}

// createJobRun routes on the job's declared trigger, not on how it happens
// to be invoked: the engine's CreateAndRun refuses a schedule-trigger spec
// outright ("create-and-run serves manual jobs only"), because registering
// a cron must never imply an immediate run. A manual-trigger job (the
// opt-out default included) still goes through CreateAndRun unchanged.
func TestCreateJobRun(t *testing.T) {
	s := &composeService{}
	project := &types.Project{Name: "myproject"}
	spec := &jobsv0.JobSpec{}

	t.Run("a manual-trigger job uses CreateAndRun", func(t *testing.T) {
		yes := true
		job := types.JobConfig{Name: "migrate", Triggers: &types.TriggerConfig{Manual: &yes}}
		fake := &fakeJobsClient{createAndRunReply: &jobsv0.CreateAndRunReply{Run: &jobsv0.Run{ID: "run-1", JobID: "job-1"}}}

		run, err := s.createJobRun(t.Context(), fake, project, "migrate", job, spec)
		assert.NilError(t, err)
		assert.Equal(t, run.ID, "run-1")
		assert.Equal(t, len(fake.createAndRunCalls), 1)
		assert.Equal(t, fake.createAndRunCalls[0].Name, "myproject.migrate")
		assert.Equal(t, len(fake.createCalls), 0, "a manual job must never call Create")
		assert.Equal(t, len(fake.runCalls), 0, "a manual job must never call the schedule-job Run path")
	})

	t.Run("a scheduled job is Run as registered, never Created or CreateAndRun", func(t *testing.T) {
		job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}}
		fake := &fakeJobsClient{runReply: &jobsv0.RunReply{Run: &jobsv0.Run{ID: "run-2", JobID: "job-2"}}}

		run, err := s.createJobRun(t.Context(), fake, project, "backup", job, nil)
		assert.NilError(t, err)
		assert.Equal(t, run.ID, "run-2")
		assert.Equal(t, len(fake.runCalls), 1)
		assert.Equal(t, fake.runCalls[0].JobRef, "myproject.backup")
		assert.Assert(t, !fake.runCalls[0].Reschedule, "a manual fire must add to the cron cadence, not replace its next occurrence")
		assert.Equal(t, len(fake.createCalls)+len(fake.createAndRunCalls), 0, "Create would arm the schedule of a job up never registered")
	})

	t.Run("a scheduled job the engine does not know says up comes first", func(t *testing.T) {
		job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}}
		fake := &fakeJobsClient{runErr: status.Error(codes.NotFound, "no such job")}

		_, err := s.createJobRun(t.Context(), fake, project, "backup", job, nil)
		assert.ErrorContains(t, err, `job "backup" is not registered with the engine`)
		assert.ErrorContains(t, err, "docker compose up")
		assert.Equal(t, len(fake.createCalls), 0)
	})
}

// TestBuildJobSpecIsDeterministic guards against the spec hash churning
// across invocations when a job declares several environment variables:
// buildJobSpec marshals the container spec as JSON and sends it to the
// engine, which identifies the job by a hash of that payload, so any
// map-to-slice conversion that doesn't sort its output (Env, Ulimits,
// ExtraHosts, mounts) makes every other `up`/`run` fail with
// `job "..." has changed`.
func TestBuildJobSpecIsDeterministic(t *testing.T) {
	s, _ := newPreStartTestService(t)
	project := &types.Project{Name: "myproject"}
	svc := types.ServiceConfig{
		Name: "backup",
		WorkloadSpec: types.WorkloadSpec{
			DependsOn: types.DependsOnConfig{
				"a": {Condition: types.ServiceConditionStarted, Required: true},
				"b": {Condition: types.ServiceConditionHealthy, Required: true},
				"c": {Condition: types.ServiceConditionStarted, Required: true},
			},
		},
		ContainerSpec: types.ContainerSpec{
			Image: "alpine",
			Environment: types.MappingWithEquals{
				"A": strPtr("1"), "B": strPtr("2"), "C": strPtr("3"), "D": strPtr("4"),
				"E": strPtr("5"), "F": strPtr("6"), "G": strPtr("7"), "H": strPtr("8"),
			},
			Ulimits: map[string]*types.UlimitsConfig{
				"nofile": {Soft: 1024, Hard: 2048},
				"nproc":  {Single: 100},
			},
			ExtraHosts: types.HostsList{
				"a.example.com": []string{"10.0.0.1"},
				"b.example.com": []string{"10.0.0.2"},
			},
			Volumes: []types.ServiceVolumeConfig{
				{Type: types.VolumeTypeVolume, Target: "/var/a"},
				{Type: types.VolumeTypeVolume, Target: "/var/b"},
			},
		},
	}
	manual := true
	job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{Manual: &manual}}

	first, err := s.buildJobSpec(t.Context(), project, svc, job, false)
	assert.NilError(t, err)

	msg := "job spec must be byte-identical across invocations, or the engine's spec hash churns and up/run intermittently fail with \"has changed\""
	for i := 0; i < 100; i++ {
		spec, err := s.buildJobSpec(t.Context(), project, svc, job, false)
		assert.NilError(t, err)
		assert.Equal(t, string(spec.ContainerSpec), string(first.ContainerSpec), msg)
	}
}

// The Compose version and the digest of the job's image stay out of its spec.
func TestBuildJobSpecIgnoresVolatileLabels(t *testing.T) {
	s, _ := newPreStartTestService(t)
	project := &types.Project{Name: "myproject", WorkingDir: "/work"}
	manual := true
	job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{Manual: &manual}}
	svc := JobAsService(project, "backup", job)
	svc.Image = "alpine"
	svc.CustomLabels[api.ImageDigestLabel] = "sha256:aaa"

	spec, err := s.buildJobSpec(t.Context(), project, svc, job, false)
	assert.NilError(t, err)

	labels := string(spec.ContainerSpec)
	assert.Assert(t, strings.Contains(labels, api.ProjectLabel), "the labels that identify the run container must stay")
	assert.Assert(t, !strings.Contains(labels, api.VersionLabel))
	assert.Assert(t, !strings.Contains(labels, api.ImageDigestLabel))
}

// up registers every scheduled job, and only those, under the project-scoped
// engine name; a dry run must not reach the real engine.
func TestRegisterScheduledJobs(t *testing.T) {
	newService := func(t *testing.T, fake *fakeJobsClient) *composeService {
		t.Helper()
		s, mockAPI := newPreStartTestServiceWithVersion(t, apiVersionJobs)
		mockAPI.EXPECT().ImageInspect(gomock.Any(), gomock.Any(), gomock.Any()).Return(client.ImageInspectResult{}, nil).AnyTimes()
		s.jobsGRPCOnce.Do(func() {})
		s.jobsAPI = fake
		return s
	}
	project := func() *types.Project {
		p := scheduledJobsProject()
		web := p.Services["web"]
		web.Image = "alpine"
		p.Services["web"] = web
		for name, job := range p.Jobs {
			job.Image = "alpine"
			p.Jobs[name] = job
		}
		return p
	}

	t.Run("registers the scheduled jobs, not the manual ones", func(t *testing.T) {
		fake := &fakeJobsClient{}
		assert.NilError(t, newService(t, fake).registerScheduledJobs(t.Context(), project()))
		var names []string
		for _, call := range fake.createCalls {
			names = append(names, call.Name)
			assert.Assert(t, call.Spec.Trigger.Schedule != nil, "%s must carry its schedule", call.Name)
			assert.Assert(t, call.Spec.RemoveOnSuccess, "%s: a scheduled job's runs are disposable", call.Name)
		}
		slices.Sort(names)
		assert.DeepEqual(t, names, []string{"prj.backup", "prj.sync"})
	})

	t.Run("a service the job shares with is registered by name, not by service", func(t *testing.T) {
		fake := &fakeJobsClient{}
		p := project()
		backup := p.Jobs["backup"]
		backup.VolumesFrom = []string{"web"}
		p.Jobs["backup"] = backup

		assert.NilError(t, newService(t, fake).registerScheduledJobs(t.Context(), p))

		var registered string
		for _, call := range fake.createCalls {
			if call.Name == "prj.backup" {
				registered = string(call.Spec.ContainerSpec)
			}
		}
		assert.Assert(t, strings.Contains(registered, `"VolumesFrom":["prj-web-1"]`), registered)
	})

	// up already pulled the services: registering the jobs must not do it again
	t.Run("the images of the services are left alone", func(t *testing.T) {
		p := project()
		web := p.Services["web"]
		web.PullPolicy = types.PullPolicyAlways
		p.Services["web"] = web

		// no ImagePull expected on the mock: one would fail the test
		assert.NilError(t, newService(t, &fakeJobsClient{}).registerScheduledJobs(t.Context(), p))
	})

	t.Run("a changed job is reported with the way out", func(t *testing.T) {
		fake := &fakeJobsClient{createErr: status.Error(codes.AlreadyExists, "spec differs")}
		err := newService(t, fake).registerScheduledJobs(t.Context(), project())
		assert.ErrorContains(t, err, "has changed: run `docker compose down`")
	})

	t.Run("a dry run registers nothing on the real engine", func(t *testing.T) {
		fake := &fakeJobsClient{}
		s := newService(t, fake)
		s.dryRun = true
		assert.NilError(t, s.registerScheduledJobs(t.Context(), project()))
		assert.Equal(t, len(fake.createCalls), 0)
	})
}

// create arms the scheduled jobs like up does: the engine's cron is the only
// scheduler, and a job that failed because the services it needs were not
// started leaves its logs for the user.
func TestCreateRegistersScheduledJobs(t *testing.T) {
	s, mockAPI := newPreStartTestServiceWithVersion(t, apiVersionJobs)
	mockAPI.EXPECT().ImageInspect(gomock.Any(), gomock.Any(), gomock.Any()).Return(client.ImageInspectResult{}, nil).AnyTimes()
	mockAPI.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{}, nil).AnyTimes()
	mockAPI.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil).AnyTimes()
	mockAPI.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{}, nil).AnyTimes()
	fake := &fakeJobsClient{}
	s.jobsGRPCOnce.Do(func() {})
	s.jobsAPI = fake

	project := scheduledJobsProject()
	project.Services = types.Services{}
	for name, job := range project.Jobs {
		job.Image = "alpine"
		project.Jobs[name] = job
	}

	assert.NilError(t, s.Create(t.Context(), project, api.CreateOptions{}))

	var names []string
	for _, call := range fake.createCalls {
		names = append(names, call.Name)
	}
	slices.Sort(names)
	assert.DeepEqual(t, names, []string{"prj.backup", "prj.sync"})
}

// create reports the jobs the file no longer declares, and removes them with
// --remove-orphans.
func TestCreateHandlesOrphanJobs(t *testing.T) {
	run := func(t *testing.T, options api.CreateOptions) (*fakeJobsClient, string) {
		t.Helper()
		out := captureWarnings(t)
		s, mockAPI := newPreStartTestServiceWithVersion(t, apiVersionJobs)
		mockAPI.EXPECT().ImageInspect(gomock.Any(), gomock.Any(), gomock.Any()).Return(client.ImageInspectResult{}, nil).AnyTimes()
		mockAPI.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{}, nil).AnyTimes()
		mockAPI.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil).AnyTimes()
		mockAPI.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{}, nil).AnyTimes()
		fake := &fakeJobsClient{listReply: &jobsv0.ListReply{Jobs: []jobsv0.Job{{Name: "prj.backup"}, {Name: "prj.sync"}, {Name: "prj.old"}}}}
		s.jobsGRPCOnce.Do(func() {})
		s.jobsAPI = fake

		project := scheduledJobsProject()
		project.Services = types.Services{}
		for name, job := range project.Jobs {
			job.Image = "alpine"
			project.Jobs[name] = job
		}
		assert.NilError(t, s.Create(t.Context(), project, options))
		return fake, out.String()
	}

	t.Run("without the flag it says so", func(t *testing.T) {
		fake, out := run(t, api.CreateOptions{})
		assert.Assert(t, strings.Contains(out, "Found orphan jobs (old)"), out)
		assert.Equal(t, len(fake.removeCalls), 0)
	})

	t.Run("with --remove-orphans it removes them", func(t *testing.T) {
		fake, out := run(t, api.CreateOptions{RemoveOrphans: true})
		assert.Assert(t, !strings.Contains(out, "orphan jobs"), out)
		assert.Equal(t, len(fake.removeCalls), 1)
		assert.Equal(t, fake.removeCalls[0].JobRef, "prj.old")
	})
}

// A scheduled job runs as `up` registered it: what `run` was asked to change in
// it, on the command line, would not reach the engine, so it is refused rather
// than ignored. The cmd layer applies -v and -p to the service itself.
func TestScheduledJobOverrides(t *testing.T) {
	job := types.JobConfig{
		Name:          "backup",
		Triggers:      &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}},
		ContainerSpec: types.ContainerSpec{Image: "alpine", Volumes: []types.ServiceVolumeConfig{{Type: types.VolumeTypeVolume, Target: "/data"}}},
		WorkloadSpec:  types.WorkloadSpec{Ports: []types.ServicePortConfig{{Target: 80}}},
	}
	project := &types.Project{Name: "prj"}
	declared := JobAsService(project, "backup", job)

	assert.Equal(t, len(scheduledJobOverrides(api.RunOptions{}, declared, job)), 0, "a run with no option is the job as registered")

	got := scheduledJobOverrides(api.RunOptions{
		Command:     []string{"sh"},
		Entrypoint:  []string{"/bin/sh"},
		User:        "root",
		WorkingDir:  "/tmp",
		Environment: []string{"X=1"},
		CapAdd:      []string{"NET_ADMIN"},
		CapDrop:     []string{"ALL"},
		Labels:      types.Labels{"a": "b"},
	}, declared, job)
	assert.DeepEqual(t, got, []string{"a command", "--entrypoint", "--user", "--workdir", "--env/--env-from-file", "--cap-add", "--cap-drop", "--label"})

	changed := declared
	changed.Volumes = append(slices.Clone(declared.Volumes), types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: "/a", Target: "/b"})
	changed.Ports = []types.ServicePortConfig{{Target: 443}}
	assert.DeepEqual(t, scheduledJobOverrides(api.RunOptions{}, changed, job), []string{"--volume", "--publish"})
}

func TestRunJobRefusesOverridesOfAScheduledJob(t *testing.T) {
	project := scheduledJobsProject()
	project.Services["backup"] = JobAsService(project, "backup", project.Jobs["backup"])
	fake := &fakeJobsClient{}

	_, err := newJobsTestService(t, apiVersionJobs, fake).RunJob(t.Context(), project, "backup",
		api.RunOptions{Service: "backup", Environment: []string{"X=1"}, User: "root"})

	assert.ErrorContains(t, err, `scheduled job "backup" runs as registered by up`)
	assert.ErrorContains(t, err, "--user, --env")
	assert.Equal(t, len(fake.createCalls)+len(fake.createAndRunCalls)+len(fake.runCalls), 0)
}

// A manual job does not outlive its run: removed from the engine, its spec is
// not held against the next `run`, which may carry other options. The run
// containers stay, Remove keeps the history.
func TestEndJobRun(t *testing.T) {
	s := &composeService{}

	t.Run("a manual job is removed once its run is over", func(t *testing.T) {
		fake := &fakeJobsClient{}
		s.endJobRun(t.Context(), fake, "migrate", "job-1", true)
		assert.Equal(t, len(fake.removeCalls), 1)
		assert.Equal(t, fake.removeCalls[0].JobRef, "job-1")
		assert.Equal(t, len(fake.cancelCalls), 0)
	})

	t.Run("an interrupted run is cancelled, then its job removed", func(t *testing.T) {
		fake := &fakeJobsClient{}
		ctx, interrupt := context.WithCancel(t.Context())
		interrupt()
		s.endJobRun(ctx, fake, "migrate", "job-1", true)
		assert.Equal(t, len(fake.cancelCalls), 1)
		assert.Equal(t, len(fake.removeCalls), 1, "the context is done, the removal still goes through")
	})

	t.Run("a scheduled job stays registered", func(t *testing.T) {
		fake := &fakeJobsClient{}
		s.endJobRun(t.Context(), fake, "backup", "job-2", false)
		assert.Equal(t, len(fake.removeCalls), 0)
	})

	t.Run("a job already gone is not worth a word, a failure is", func(t *testing.T) {
		out := captureWarnings(t)
		s.endJobRun(t.Context(), &fakeJobsClient{removeErr: status.Error(codes.NotFound, "no such job")}, "migrate", "job-1", true)
		assert.Equal(t, out.String(), "")

		s.endJobRun(t.Context(), &fakeJobsClient{removeErr: errors.New("engine unreachable")}, "migrate", "job-1", true)
		assert.Assert(t, strings.Contains(out.String(), "engine unreachable"), out.String())
	})
}

// Interrupting `run` only detaches its Wait: the engine keeps running the job
// unless it is told to stop, so an interrupted run cancels its run, even though
// the context that carried the interruption is already done.
func TestCancelInterruptedJobRun(t *testing.T) {
	t.Run("a run that ended on its own is left alone", func(t *testing.T) {
		fake := &fakeJobsClient{}
		cancelInterruptedJobRun(t.Context(), fake, "migrate", "job-1")
		assert.Equal(t, len(fake.cancelCalls), 0)
	})

	t.Run("an interrupted run is cancelled", func(t *testing.T) {
		fake := &fakeJobsClient{}
		ctx, interrupt := context.WithCancel(t.Context())
		interrupt()
		cancelInterruptedJobRun(ctx, fake, "migrate", "job-1")
		assert.Equal(t, len(fake.cancelCalls), 1)
		assert.Equal(t, fake.cancelCalls[0].JobRef, "job-1")
	})

	t.Run("a failed cancel is reported, the job keeps running", func(t *testing.T) {
		warnings := captureWarnings(t)
		fake := &fakeJobsClient{cancelErr: errors.New("engine unreachable")}
		ctx, interrupt := context.WithCancel(t.Context())
		interrupt()
		cancelInterruptedJobRun(ctx, fake, "migrate", "job-1")
		assert.Assert(t, strings.Contains(warnings.String(), "engine unreachable"), warnings.String())
	})
}

// The engine drops a scheduled job's container as soon as a run succeeds; a
// manual job's is kept so RunJob can read its logs. The policy is a function of
// the job alone: `run` must send a scheduled job exactly the spec up registered.
func TestBuildJobSpecRemoveOnSuccess(t *testing.T) {
	s, _ := newPreStartTestService(t)
	project := &types.Project{Name: "myproject"}
	svc := types.ServiceConfig{Name: "backup", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	manual := true

	for name, tc := range map[string]struct {
		job  types.JobConfig
		want bool
	}{
		"manual":    {job: types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{Manual: &manual}}, want: false},
		"scheduled": {job: types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}}}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			spec, err := s.buildJobSpec(t.Context(), project, svc, tc.job, false)
			assert.NilError(t, err)
			assert.Equal(t, spec.RemoveOnSuccess, tc.want)
		})
	}
}

// scheduledJobsProject declares one scheduled and one manual job next to a
// service, the shape the degraded-mode tests below run against.
func scheduledJobsProject() *types.Project {
	yes := true
	return &types.Project{
		Name:     "prj",
		Services: types.Services{"web": {Name: "web"}},
		Jobs: types.Jobs{
			"backup":  {Name: "backup", Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}}},
			"sync":    {Name: "sync", Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 4 * * *"}}}},
			"migrate": {Name: "migrate", Triggers: &types.TriggerConfig{Manual: &yes}},
		},
	}
}

// newJobsTestService returns a compose service whose engine negotiates
// apiVersion and whose jobs extension is the given fake. The mocks carry no
// other expectation: any daemon call beyond the version negotiation fails the
// test.
func newJobsTestService(t *testing.T, apiVersion string, fake *fakeJobsClient) *composeService {
	t.Helper()
	_, tested := newTestComposeService(t, gomock.NewController(t), apiVersion)
	tested.jobsGRPCOnce.Do(func() {}) // keep jobsClient from dialing: the fake stands in for the extension
	tested.jobsAPI = fake
	return tested
}

var (
	// jobsExtensionOffErr is what an engine whose jobs extension is not
	// enabled answers to any call of the jobs service.
	jobsExtensionOffErr = status.Error(codes.Unimplemented, "unknown service org.mobyproject.extension.jobs.api.v0.Jobs")
	// jobsExtensionNotReadyErr is what an engine whose jobs extension is
	// enabled but still reconciling (or failed to activate) answers.
	jobsExtensionNotReadyErr = status.Error(codes.Unavailable, "the jobs extension is not active")
)

const (
	// noJobsEngine is the newest API version below apiVersionJobs.
	noJobsEngine = "1.56"
	// scheduledJobsRefused is what a project declaring the scheduled jobs of
	// scheduledJobsProject gets on an engine whose API predates jobs.
	scheduledJobsRefused = "scheduled jobs are not supported by this version of the engine: backup, sync"
	// scheduledJobsExtensionOffRefused is what it gets on an engine that has
	// the jobs API but not the extension enabled: it names the flag to start.
	scheduledJobsExtensionOffRefused = "scheduled jobs need the engine's jobs extension, which is not enabled " +
		"(start dockerd with --extension-enable org.mobyproject.jobs.v0): backup, sync"
	// jobsExtensionNotReady prefixes the distinct error of an enabled but not
	// ready extension.
	jobsExtensionNotReady = "the engine's jobs extension is enabled but not ready: the jobs extension is not active"
)

func TestJobsAvailability(t *testing.T) {
	t.Run("an engine whose API predates jobs is too old, without any gRPC call", func(t *testing.T) {
		for _, version := range []string{"1.44", noJobsEngine} {
			fake := &fakeJobsClient{}
			got, err := newJobsTestService(t, version, fake).jobsAvailability(t.Context(), "prj")
			assert.NilError(t, err)
			assert.Equal(t, got, jobsEngineTooOld, version)
			assert.Equal(t, len(fake.listCalls), 0, "the API version alone must decide, no probe")
		}
	})

	t.Run("an engine answering Unimplemented has the extension off", func(t *testing.T) {
		fake := &fakeJobsClient{listErr: jobsExtensionOffErr}
		got, err := newJobsTestService(t, apiVersionJobs, fake).jobsAvailability(t.Context(), "prj")
		assert.NilError(t, err)
		assert.Equal(t, got, jobsExtensionOff)
	})

	t.Run("an engine answering the probe, even with no job, has the extension, probed with a List scoped to the project", func(t *testing.T) {
		fake := &fakeJobsClient{}
		got, err := newJobsTestService(t, apiVersionJobs, fake).jobsAvailability(t.Context(), "prj")
		assert.NilError(t, err)
		assert.Equal(t, got, jobsAvailable)
		assert.Equal(t, len(fake.listCalls), 1)
		assert.DeepEqual(t, fake.listCalls[0].Labels, []string{api.ProjectLabel + "=prj"})
	})

	t.Run("Unavailable is a distinct error, never an availability", func(t *testing.T) {
		fake := &fakeJobsClient{listErr: jobsExtensionNotReadyErr}
		_, err := newJobsTestService(t, apiVersionJobs, fake).jobsAvailability(t.Context(), "prj")
		assert.ErrorIs(t, err, errJobsExtensionNotReady)
		assert.ErrorContains(t, err, jobsExtensionNotReady)
	})

	t.Run("any other error is returned as is", func(t *testing.T) {
		boom := errors.New("connection refused")
		fake := &fakeJobsClient{listErr: boom}
		_, err := newJobsTestService(t, apiVersionJobs, fake).jobsAvailability(t.Context(), "prj")
		assert.ErrorIs(t, err, boom)
		assert.Assert(t, !errors.Is(err, errJobsExtensionNotReady))
	})

	t.Run("a definitive answer is probed once per process", func(t *testing.T) {
		for _, listErr := range []error{nil, jobsExtensionOffErr} {
			fake := &fakeJobsClient{listErr: listErr}
			tested := newJobsTestService(t, apiVersionJobs, fake)
			first, err := tested.jobsAvailability(t.Context(), "prj")
			assert.NilError(t, err)
			second, err := tested.jobsAvailability(t.Context(), "prj")
			assert.NilError(t, err)
			assert.Equal(t, first, second)
			assert.Equal(t, len(fake.listCalls), 1)
		}
	})

	t.Run("an error is never memoized: the extension may be ready on the next call", func(t *testing.T) {
		fake := &fakeJobsClient{listErr: jobsExtensionNotReadyErr}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		_, err := tested.jobsAvailability(t.Context(), "prj")
		assert.ErrorIs(t, err, errJobsExtensionNotReady)

		fake.listErr = nil
		got, err := tested.jobsAvailability(t.Context(), "prj")
		assert.NilError(t, err)
		assert.Equal(t, got, jobsAvailable)
		assert.Equal(t, len(fake.listCalls), 2)
	})
}

func TestRejectUnsupportedScheduledJobs(t *testing.T) {
	t.Run("an engine with the extension accepts them", func(t *testing.T) {
		err := newJobsTestService(t, apiVersionJobs, &fakeJobsClient{}).rejectUnsupportedScheduledJobs(t.Context(), scheduledJobsProject())
		assert.NilError(t, err)
	})

	t.Run("manual jobs never block nor probe, whatever the engine", func(t *testing.T) {
		project := scheduledJobsProject()
		delete(project.Jobs, "backup")
		delete(project.Jobs, "sync")
		fake := &fakeJobsClient{listErr: jobsExtensionOffErr}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).rejectUnsupportedScheduledJobs(t.Context(), project))
		assert.Equal(t, len(fake.listCalls), 0)
	})

	t.Run("profile-disabled scheduled jobs don't block", func(t *testing.T) {
		project := &types.Project{DisabledJobs: scheduledJobsProject().Jobs}
		assert.NilError(t, newJobsTestService(t, noJobsEngine, &fakeJobsClient{}).rejectUnsupportedScheduledJobs(t.Context(), project))
	})

	t.Run("a project without jobs never negotiates the engine version", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		_, cli := prepareMocks(mockCtrl) // no Ping expectation: negotiating would fail
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		assert.NilError(t, tested.(*composeService).rejectUnsupportedScheduledJobs(t.Context(), &types.Project{Name: "prj"}))
	})
}

// Without usable jobs support, create, up and start refuse a project
// declaring scheduled jobs before touching the daemon: the mocks carry no
// expectation besides the version negotiation.
func TestScheduledJobsRefusedWithoutJobsEngine(t *testing.T) {
	for name, tc := range map[string]struct {
		version string
		listErr error
		refused string
	}{
		"API predates jobs":   {version: noJobsEngine, refused: scheduledJobsRefused},
		"extension off":       {version: apiVersionJobs, listErr: jobsExtensionOffErr, refused: scheduledJobsExtensionOffRefused},
		"extension not ready": {version: apiVersionJobs, listErr: jobsExtensionNotReadyErr, refused: jobsExtensionNotReady},
	} {
		t.Run(name, func(t *testing.T) {
			tested := newJobsTestService(t, tc.version, &fakeJobsClient{listErr: tc.listErr})
			project := scheduledJobsProject()

			assert.ErrorContains(t, tested.Create(t.Context(), project, api.CreateOptions{}), tc.refused)
			assert.ErrorContains(t, tested.Up(t.Context(), project, api.UpOptions{}), tc.refused)
			assert.ErrorContains(t, tested.Start(t.Context(), project.Name, api.StartOptions{Project: project}), tc.refused)
		})
	}
}

// RunJob reports the missing engine support with a sentinel before doing any
// work, so the CLI can fall back to running the job like a one-off service:
// the error says why. An extension that is enabled but not ready is not
// missing support, it is a failure: no sentinel, no fallback.
func TestRunJobWithoutJobsEngine(t *testing.T) {
	run := func(t *testing.T, version string, listErr error) error {
		t.Helper()
		tested := newJobsTestService(t, version, &fakeJobsClient{listErr: listErr})
		_, err := tested.RunJob(t.Context(), scheduledJobsProject(), "migrate", api.RunOptions{Service: "migrate"})
		return err
	}

	t.Run("API predates jobs", func(t *testing.T) {
		err := run(t, noJobsEngine, nil)
		assert.ErrorIs(t, err, api.ErrJobsNotSupported)
		assert.ErrorContains(t, err, "API version "+noJobsEngine)
	})

	t.Run("extension off", func(t *testing.T) {
		err := run(t, apiVersionJobs, jobsExtensionOffErr)
		assert.ErrorIs(t, err, api.ErrJobsNotSupported)
		assert.ErrorContains(t, err, "--extension-enable org.mobyproject.jobs.v0")
	})

	t.Run("extension not ready", func(t *testing.T) {
		err := run(t, apiVersionJobs, jobsExtensionNotReadyErr)
		assert.ErrorIs(t, err, errJobsExtensionNotReady)
		assert.Assert(t, !errors.Is(err, api.ErrJobsNotSupported), "no sentinel: the CLI must not degrade to a one-off")
	})
}

// captureWarnings captures what the code under test tells the user.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logrus.SetOutput(&buf)
	t.Cleanup(func() { logrus.SetOutput(os.Stderr) })
	return &buf
}

// A job runs until it is done and its run is not simulated by a dry run: the
// jobs gRPC dials the real engine, bypassing the dry-run client.
func TestRunJobDetachAndDryRun(t *testing.T) {
	options := api.RunOptions{Service: "migrate"}

	t.Run("a job cannot be detached", func(t *testing.T) {
		fake := &fakeJobsClient{}
		options := options
		options.Detach = true
		_, err := newJobsTestService(t, apiVersionJobs, fake).RunJob(t.Context(), scheduledJobsProject(), "migrate", options)
		assert.ErrorContains(t, err, "--detach is not supported for a job")
		assert.Equal(t, len(fake.createAndRunCalls), 0)
	})

	t.Run("a degraded run keeps honouring detach: it is a one-off service", func(t *testing.T) {
		options := options
		options.Detach = true
		_, err := newJobsTestService(t, noJobsEngine, &fakeJobsClient{}).RunJob(t.Context(), scheduledJobsProject(), "migrate", options)
		assert.ErrorIs(t, err, api.ErrJobsNotSupported)
	})

	t.Run("a dry run starts nothing on the real engine", func(t *testing.T) {
		fake := &fakeJobsClient{}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		tested.dryRun = true
		code, err := tested.RunJob(t.Context(), scheduledJobsProject(), "migrate", options)
		assert.NilError(t, err)
		assert.Equal(t, code, 0)
		assert.Equal(t, len(fake.createAndRunCalls)+len(fake.createCalls)+len(fake.runCalls), 0)
	})
}

// Down only talks to the engine's jobs registry when the engine has one.
func TestDownJobsWithoutJobsEngine(t *testing.T) {
	project := scheduledJobsProject()

	t.Run("an API that predates jobs leaves nothing to remove, silently", func(t *testing.T) {
		out := captureWarnings(t)
		tested := newJobsTestService(t, noJobsEngine, &fakeJobsClient{})
		assert.Equal(t, len(tested.ensureJobsDown(t.Context(), project)), 0)
		assert.Equal(t, out.String(), "")
	})

	t.Run("an extension that is off leaves nothing to remove, silently", func(t *testing.T) {
		out := captureWarnings(t)
		tested := newJobsTestService(t, apiVersionJobs, &fakeJobsClient{listErr: jobsExtensionOffErr})
		assert.Equal(t, len(tested.ensureJobsDown(t.Context(), project)), 0)
		assert.Equal(t, out.String(), "")
	})

	t.Run("an extension enabled but not ready leaves nothing to remove, with a warning", func(t *testing.T) {
		out := captureWarnings(t)
		tested := newJobsTestService(t, apiVersionJobs, &fakeJobsClient{listErr: jobsExtensionNotReadyErr})
		assert.Equal(t, len(tested.ensureJobsDown(t.Context(), project)), 0, "down must carry on cleaning the rest of the project")
		assert.Assert(t, strings.Contains(out.String(), "level=warning"), out.String())
		assert.Assert(t, strings.Contains(out.String(), jobsExtensionNotReady), out.String())
	})

	// a manual job left behind by a `run` that was killed before it could remove
	// it goes too, with the others
	t.Run("with the extension the jobs are removed from the engine", func(t *testing.T) {
		fake := &fakeJobsClient{}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		ops := tested.ensureJobsDown(t.Context(), project)
		assert.Equal(t, len(ops), 3)
		for _, op := range ops {
			assert.NilError(t, op())
		}
		var refs []string
		for _, call := range fake.removeCalls {
			refs = append(refs, call.JobRef)
		}
		slices.Sort(refs)
		assert.DeepEqual(t, refs, []string{"prj.backup", "prj.migrate", "prj.sync"})
	})

	// without a compose file the jobs come from the engine's registry, with no
	// trigger to tell: all of them go
	t.Run("a job known by its name only is removed", func(t *testing.T) {
		fake := &fakeJobsClient{}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		ops := tested.ensureJobsDown(t.Context(), &types.Project{Name: "prj", Jobs: types.Jobs{"old": {Name: "old"}}})
		assert.Equal(t, len(ops), 1)
		assert.NilError(t, ops[0]())
		assert.Equal(t, fake.removeCalls[0].JobRef, "prj.old")
	})

	t.Run("a dry run removes nothing from the real engine", func(t *testing.T) {
		fake := &fakeJobsClient{}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		tested.dryRun = true
		for _, op := range tested.ensureJobsDown(t.Context(), project) {
			assert.NilError(t, op())
		}
		assert.Equal(t, len(fake.removeCalls), 0)
	})
}

// down without a compose file rebuilds the project's jobs from the engine's
// registry; it tells about jobs it cannot reach only when the extension is
// enabled but not ready, exactly like down with a compose file.
func TestActualJobs(t *testing.T) {
	t.Run("an API that predates jobs lists nothing, silently, without probing", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{}
		assert.Equal(t, len(newJobsTestService(t, noJobsEngine, fake).actualJobs(t.Context(), "prj")), 0)
		assert.Equal(t, len(fake.listCalls), 0)
		assert.Equal(t, out.String(), "")
	})

	t.Run("an extension that is off lists nothing, silently", func(t *testing.T) {
		out := captureWarnings(t)
		tested := newJobsTestService(t, apiVersionJobs, &fakeJobsClient{listErr: jobsExtensionOffErr})
		assert.Equal(t, len(tested.actualJobs(t.Context(), "prj")), 0)
		assert.Equal(t, out.String(), "")
	})

	t.Run("an extension enabled but not ready lists nothing, with a warning", func(t *testing.T) {
		out := captureWarnings(t)
		tested := newJobsTestService(t, apiVersionJobs, &fakeJobsClient{listErr: jobsExtensionNotReadyErr})
		assert.Equal(t, len(tested.actualJobs(t.Context(), "prj")), 0)
		assert.Assert(t, strings.Contains(out.String(), "level=warning"), out.String())
		assert.Assert(t, strings.Contains(out.String(), jobsExtensionNotReady), out.String())
	})

	t.Run("with the extension the project's jobs come back under their local names", func(t *testing.T) {
		fake := &fakeJobsClient{listReply: &jobsv0.ListReply{Jobs: []jobsv0.Job{{Name: "prj.backup"}, {Name: "prj.sync"}}}}
		got := newJobsTestService(t, apiVersionJobs, fake).actualJobs(t.Context(), "prj")
		assert.DeepEqual(t, sortedJobNames(got, nil), []string{"backup", "sync"})
	})
}

// The engine removes a quick job's container, logs included, as soon as it
// succeeds: losing them must be said, anything else going wrong while
// following them stays a debug detail.
func TestReportJobLogsError(t *testing.T) {
	t.Run("a container gone before its logs were read is a warning", func(t *testing.T) {
		out := captureWarnings(t)
		reportJobLogsError("backup", errdefs.ErrNotFound.WithMessage("No such container: abc"))
		assert.Assert(t, strings.Contains(out.String(), "level=warning"), out.String())
		assert.Assert(t, strings.Contains(out.String(), "its logs are gone"), out.String())
	})

	t.Run("any other error stays silent", func(t *testing.T) {
		out := captureWarnings(t)
		reportJobLogsError("backup", errors.New("stream reset"))
		assert.Equal(t, out.String(), "")
	})
}

// Failing to remove a manual job's kept container is worth a warning, not a
// failed run.
func TestRemoveJobRunContainer(t *testing.T) {
	removal := client.ContainerRemoveOptions{RemoveVolumes: true}

	t.Run("removes the run container", func(t *testing.T) {
		mockAPI, tested := newTestComposeService(t, gomock.NewController(t), apiVersionJobs)
		mockAPI.EXPECT().ContainerRemove(gomock.Any(), "abc", removal).Return(client.ContainerRemoveResult{}, nil)
		tested.removeJobRunContainer(t.Context(), "backup", "abc")
	})

	t.Run("a failed removal is a warning", func(t *testing.T) {
		out := captureWarnings(t)
		mockAPI, tested := newTestComposeService(t, gomock.NewController(t), apiVersionJobs)
		mockAPI.EXPECT().ContainerRemove(gomock.Any(), "abc", removal).Return(client.ContainerRemoveResult{}, errors.New("boom"))
		tested.removeJobRunContainer(t.Context(), "backup", "abc")
		assert.Assert(t, strings.Contains(out.String(), "level=warning"), out.String())
		assert.Assert(t, strings.Contains(out.String(), "abc"), out.String())
	})
}

func TestRunContainerID(t *testing.T) {
	t.Run("a container already created is used without waiting", func(t *testing.T) {
		fake := &fakeJobsClient{}
		id, err := runContainerID(t.Context(), fake, &jobsv0.Run{ID: "r1", ContainerID: "abc"})
		assert.NilError(t, err)
		assert.Equal(t, id, "abc")
		assert.Equal(t, len(fake.waitCalls), 0)
	})

	t.Run("a container not created yet is awaited by waiting for the run to leave pending", func(t *testing.T) {
		fake := &fakeJobsClient{waitReply: &jobsv0.WaitReply{Run: &jobsv0.Run{ContainerID: "def"}}}
		id, err := runContainerID(t.Context(), fake, &jobsv0.Run{ID: "r1", JobID: "j1"})
		assert.NilError(t, err)
		assert.Equal(t, id, "def")
		assert.Equal(t, len(fake.waitCalls), 1)
		assert.Equal(t, fake.waitCalls[0].Condition, jobsv0.WaitConditionRunning)
		assert.Equal(t, fake.waitCalls[0].RunRef, "r1")
	})

	t.Run("a run with no container says why its output is missing", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{waitReply: &jobsv0.WaitReply{}}
		id, err := runContainerID(t.Context(), fake, &jobsv0.Run{ID: "r1", JobID: "j1"})
		assert.NilError(t, err)
		assert.Equal(t, id, "")
		assert.Assert(t, strings.Contains(out.String(), "r1"), out.String())
		assert.Assert(t, strings.Contains(out.String(), "no container"), out.String())
	})

	t.Run("a wait failure is returned", func(t *testing.T) {
		fake := &fakeJobsClient{waitErr: status.Error(codes.NotFound, "no such run")}
		_, err := runContainerID(t.Context(), fake, &jobsv0.Run{ID: "r1"})
		assert.Assert(t, errdefs.IsNotFound(err), err)
	})
}

// run ends with the exit code a one-off container's exit status would give.
func TestJobRunExit(t *testing.T) {
	for name, tc := range map[string]struct {
		run     *jobsv0.Run
		code    int
		wantErr string
	}{
		"succeeded":                {run: &jobsv0.Run{State: jobsv0.RunStateSucceeded}},
		"failed with an exit code": {run: &jobsv0.Run{State: jobsv0.RunStateFailed, ExitCode: &jobsv0.ExitCode{Value: 3}}, code: 3},
		"failed without exit code": {run: &jobsv0.Run{ID: "r1", State: jobsv0.RunStateFailed, Error: "oom"}, code: 1, wantErr: `job "backup" run r1 failed: oom`},
		"timed out":                {run: &jobsv0.Run{State: jobsv0.RunStateTimedOut}, code: 124, wantErr: `job "backup" timed out`},
		"cancelled":                {run: &jobsv0.Run{State: jobsv0.RunStateCancelled}, code: 130, wantErr: `job "backup" was cancelled`},
		"unexpected state":         {run: &jobsv0.Run{ID: "r2", State: "bogus"}, code: 1, wantErr: `job "backup" run r2 ended in unexpected state "bogus"`},
	} {
		t.Run(name, func(t *testing.T) {
			code, err := jobRunExit("backup", tc.run)
			assert.Equal(t, code, tc.code)
			if tc.wantErr == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.wantErr)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

// A job names the container of the service it shares with, not its ID.
func TestResolveJobServiceReferences(t *testing.T) {
	project := &types.Project{Name: "prj", Services: types.Services{
		"web": {Name: "web"},
		"db":  {Name: "db", ContainerName: "the-db"},
	}}

	t.Run("services become the name of their first container", func(t *testing.T) {
		volumesFrom := []string{"web", "container:other", "db"}
		job := types.ServiceConfig{Name: "backup", ContainerSpec: types.ContainerSpec{
			VolumesFrom: volumesFrom,
			NetworkMode: "service:web",
			Ipc:         "service:db",
			Pid:         "service:web",
		}}

		assert.NilError(t, resolveJobServiceReferences(project, &job))

		assert.DeepEqual(t, job.VolumesFrom, []string{"prj-web-1", "other", "the-db"})
		assert.Equal(t, job.NetworkMode, "container:prj-web-1")
		assert.Equal(t, job.Ipc, "container:the-db")
		assert.Equal(t, job.Pid, "container:prj-web-1")
		assert.DeepEqual(t, volumesFrom, []string{"web", "container:other", "db"}) // the job of the project keeps its own
	})

	// run --no-deps and up <services> narrow the project: the service the job
	// shares with is then disabled, not gone
	t.Run("a service the selection left out resolves like the others", func(t *testing.T) {
		narrowed := &types.Project{Name: "prj", Services: types.Services{}, DisabledServices: types.Services{"db": {Name: "db"}}}
		job := types.ServiceConfig{Name: "backup", ContainerSpec: types.ContainerSpec{VolumesFrom: []string{"db"}}}

		assert.NilError(t, resolveJobServiceReferences(narrowed, &job))
		assert.DeepEqual(t, job.VolumesFrom, []string{"prj-db-1"})
	})

	t.Run("a service the project does not have is an error", func(t *testing.T) {
		job := types.ServiceConfig{Name: "backup", ContainerSpec: types.ContainerSpec{VolumesFrom: []string{"nope"}}}
		assert.ErrorContains(t, resolveJobServiceReferences(project, &job), "nope")
	})
}

// A job the engine still has for the project, which the compose file no longer
// declares, keeps firing: up says so, and --remove-orphans removes it, as it does
// the containers of a service that left the file.
func TestHandleOrphanJobs(t *testing.T) {
	registered := &jobsv0.ListReply{Jobs: []jobsv0.Job{
		{Name: "prj.backup"}, {Name: "prj.sync"}, {Name: "prj.migrate"}, {Name: "prj.off"}, {Name: "prj.old"},
	}}
	project := func() *types.Project {
		p := scheduledJobsProject()
		p.DisabledJobs = types.Jobs{"off": {Name: "off"}} // declared, out of the profiles of this run
		return p
	}

	t.Run("up names the jobs the file no longer declares", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{listReply: registered}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).handleOrphanJobs(t.Context(), project(), false, false))
		assert.Assert(t, strings.Contains(out.String(), "Found orphan jobs (old)"), out.String())
		assert.Assert(t, strings.Contains(out.String(), "--remove-orphans"), out.String())
		assert.Equal(t, len(fake.removeCalls), 0)
	})

	t.Run("IgnoreOrphans keeps it quiet", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{listReply: registered}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).handleOrphanJobs(t.Context(), project(), false, true))
		assert.Equal(t, out.String(), "")
	})

	t.Run("--remove-orphans removes them, and only them", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{listReply: registered}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).handleOrphanJobs(t.Context(), project(), true, false))
		assert.Equal(t, len(fake.removeCalls), 1)
		assert.Equal(t, fake.removeCalls[0].JobRef, "prj.old")
		assert.Equal(t, out.String(), "")
	})

	t.Run("a dry run removes nothing", func(t *testing.T) {
		fake := &fakeJobsClient{listReply: registered}
		tested := newJobsTestService(t, apiVersionJobs, fake)
		tested.dryRun = true
		assert.NilError(t, tested.handleOrphanJobs(t.Context(), project(), true, false))
		assert.Equal(t, len(fake.removeCalls), 0)
	})

	// the registry is looked at when it can matter: a project that declares
	// scheduled jobs, or one that was asked to clean up
	t.Run("a project with no scheduled job does not look at the registry", func(t *testing.T) {
		out := captureWarnings(t)
		fake := &fakeJobsClient{listReply: registered}
		p := &types.Project{Name: "prj", Services: types.Services{"web": {Name: "web"}}}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).handleOrphanJobs(t.Context(), p, false, false))
		assert.Equal(t, len(fake.listCalls), 0)
		assert.Equal(t, out.String(), "")
	})

	t.Run("--remove-orphans looks even when the file declares no scheduled job", func(t *testing.T) {
		fake := &fakeJobsClient{listReply: registered}
		p := &types.Project{Name: "prj", Services: types.Services{"web": {Name: "web"}}}
		assert.NilError(t, newJobsTestService(t, apiVersionJobs, fake).handleOrphanJobs(t.Context(), p, true, false))
		assert.Equal(t, len(fake.removeCalls), 5)
	})
}
