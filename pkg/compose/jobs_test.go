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

	"github.com/compose-spec/compose-go/v2/types"
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

	// A job declaring both manual:true and a schedule used to silently lose
	// the schedule: the switch checked Manual before Schedule, so
	// registerScheduledJobs would still select the job (HasSchedule doesn't
	// look at Manual) but jobTrigger built a Manual-only Trigger, dropping
	// the cron with no error and no warning anywhere.
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
	assert.DeepEqual(t, sortedJobNames(types.Jobs{}, HasSchedule), []string{})
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
		Name: "myproject",
		Services: types.Services{
			"db": {
				Name: "db",
				ContainerSpec: types.ContainerSpec{
					Image:        "postgres",
					Environment:  types.MappingWithEquals{"FOO": strPtr("original")},
					CustomLabels: types.Labels{"original": "label"},
				},
			},
		},
		Jobs: types.Jobs{
			"backup": {
				Name:          "backup",
				Extensions:    types.Extensions{"x-team": "platform"},
				ContainerSpec: types.ContainerSpec{Image: "backup-tool"},
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

	t.Run("existing services are carried over", func(t *testing.T) {
		svc, err := scoped.GetService("db")
		assert.NilError(t, err)
		assert.Equal(t, svc.Image, "postgres")
	})

	t.Run("the real project.Services is never mutated", func(t *testing.T) {
		_, ok := project.Services["backup"]
		assert.Assert(t, !ok, "the job must not leak into the shared project's Services")
		assert.Equal(t, len(project.Services), 1)
	})

	t.Run("Configs is its own map, not shared with the real project", func(t *testing.T) {
		scoped.Configs["new"] = types.ConfigObjConfig{Content: "added"}
		_, ok := project.Configs["new"]
		assert.Assert(t, !ok, "writing to the scoped copy's Configs must not be visible on the shared project — registerScheduledJobs runs this concurrently per job")
		assert.Equal(t, project.Configs["cfg"].Content, "original")
	})

	t.Run("a carried-over service's Environment is its own map, not shared with the real project", func(t *testing.T) {
		svc, err := scoped.GetService("db")
		assert.NilError(t, err)
		svc.Environment["FOO"] = strPtr("mutated")
		// useAPISocket/ensureModels write into a job's scoped Environment map
		// concurrently with other jobs' registration — it must not be the
		// shared project's map.
		assert.Equal(t, *project.Services["db"].Environment["FOO"], "original")
	})

	t.Run("a carried-over service's CustomLabels is its own map, not shared with the real project", func(t *testing.T) {
		svc, err := scoped.GetService("db")
		assert.NilError(t, err)
		svc.CustomLabels["new"] = "added"
		_, ok := project.Services["db"].CustomLabels["new"]
		assert.Assert(t, !ok, "ensureImagesExists writes into a job's scoped CustomLabels map (via Labels.Add) concurrently with other jobs' registration — it must not be the shared project's map")
	})
}

// fakeJobsClient is a minimal jobsv0.Jobs double for createJobRun's routing:
// only Create/Run/CreateAndRun are ever exercised by RunJob, so every other
// method is left to the embedded nil interface — calling one would panic,
// which is exactly the loud failure a test relying on it deserves.
type fakeJobsClient struct {
	jobsv0.Jobs

	createCalls       []*jobsv0.CreateRequest
	createErr         error
	createAndRunCalls []*jobsv0.CreateAndRunRequest
	createAndRunReply *jobsv0.CreateAndRunReply
	createAndRunErr   error
	runCalls          []*jobsv0.RunRequest
	runReply          *jobsv0.RunReply
	runErr            error
}

func (f *fakeJobsClient) Create(_ context.Context, req *jobsv0.CreateRequest) (*jobsv0.CreateReply, error) {
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

	t.Run("a scheduled job uses Create then Run, not CreateAndRun", func(t *testing.T) {
		job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}}
		fake := &fakeJobsClient{runReply: &jobsv0.RunReply{Run: &jobsv0.Run{ID: "run-2", JobID: "job-2"}}}

		run, err := s.createJobRun(t.Context(), fake, project, "backup", job, spec)
		assert.NilError(t, err)
		assert.Equal(t, run.ID, "run-2")
		assert.Equal(t, len(fake.createCalls), 1)
		assert.Equal(t, fake.createCalls[0].Name, "myproject.backup")
		assert.Equal(t, len(fake.runCalls), 1)
		assert.Equal(t, fake.runCalls[0].JobRef, "myproject.backup")
		assert.Assert(t, !fake.runCalls[0].Reschedule, "a manual fire must add to the cron cadence, not replace its next occurrence")
		assert.Equal(t, len(fake.createAndRunCalls), 0, "a scheduled job must never call CreateAndRun: the engine rejects it")
	})

	t.Run("a scheduled job already registered by up is a no-op Create, not a conflict", func(t *testing.T) {
		job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}}
		fake := &fakeJobsClient{runReply: &jobsv0.RunReply{Run: &jobsv0.Run{ID: "run-3"}}}

		_, err := s.createJobRun(t.Context(), fake, project, "backup", job, spec)
		assert.NilError(t, err, "Create must succeed as a no-op when up already registered the identical spec")
	})

	t.Run("a scheduled job with a changed spec reports the same conflict a manual job would", func(t *testing.T) {
		job := types.JobConfig{Name: "backup", Triggers: &types.TriggerConfig{
			Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
		}}
		fake := &fakeJobsClient{createErr: status.Error(codes.AlreadyExists, "spec differs")}

		_, err := s.createJobRun(t.Context(), fake, project, "backup", job, spec)
		assert.Error(t, err, `job "backup" has changed: run `+"`docker compose down`"+` to remove it, then `+"`run`"+` again`)
		assert.Equal(t, len(fake.runCalls), 0, "Run must not be attempted after a Create conflict")
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
// apiVersion. The mocks carry no other expectation: any daemon call beyond
// the version negotiation fails the test.
func newJobsTestService(t *testing.T, apiVersion string) *composeService {
	t.Helper()
	_, tested := newTestComposeService(t, gomock.NewController(t), apiVersion)
	return tested
}

const (
	// noJobsEngine is the newest API version below apiVersionJobs.
	noJobsEngine = "1.56"
	// scheduledJobsRefused is what a project declaring the scheduled jobs of
	// scheduledJobsProject gets on an engine without the jobs API.
	scheduledJobsRefused = "scheduled jobs are not supported in this version: backup, sync"
)

func TestJobsSupported(t *testing.T) {
	for version, want := range map[string]bool{
		"1.44":         false,
		noJobsEngine:   false,
		apiVersionJobs: true,
		"1.96":         true,
	} {
		t.Run(version, func(t *testing.T) {
			supported, err := newJobsTestService(t, version).jobsSupported(t.Context())
			assert.NilError(t, err)
			assert.Equal(t, supported, want)
		})
	}
}

func TestRejectUnsupportedScheduledJobs(t *testing.T) {
	t.Run("an engine without the jobs API refuses scheduled jobs, naming them", func(t *testing.T) {
		err := newJobsTestService(t, noJobsEngine).rejectUnsupportedScheduledJobs(t.Context(), scheduledJobsProject())
		assert.Error(t, err, scheduledJobsRefused)
	})

	t.Run("an engine with the jobs API accepts them", func(t *testing.T) {
		err := newJobsTestService(t, apiVersionJobs).rejectUnsupportedScheduledJobs(t.Context(), scheduledJobsProject())
		assert.NilError(t, err)
	})

	t.Run("manual jobs never block, whatever the engine", func(t *testing.T) {
		project := scheduledJobsProject()
		delete(project.Jobs, "backup")
		delete(project.Jobs, "sync")
		assert.NilError(t, newJobsTestService(t, noJobsEngine).rejectUnsupportedScheduledJobs(t.Context(), project))
	})

	t.Run("profile-disabled scheduled jobs don't block", func(t *testing.T) {
		project := &types.Project{DisabledJobs: scheduledJobsProject().Jobs}
		assert.NilError(t, newJobsTestService(t, noJobsEngine).rejectUnsupportedScheduledJobs(t.Context(), project))
	})

	t.Run("a project without jobs never negotiates the engine version", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		_, cli := prepareMocks(mockCtrl) // no Ping expectation: negotiating would fail
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		assert.NilError(t, tested.(*composeService).rejectUnsupportedScheduledJobs(t.Context(), &types.Project{Name: "prj"}))
	})
}

// Without the jobs API, create, up and start refuse a project declaring
// scheduled jobs before touching the daemon: the mocks carry no expectation
// besides the version negotiation.
func TestScheduledJobsRefusedWithoutJobsEngine(t *testing.T) {
	tested := newJobsTestService(t, noJobsEngine)
	project := scheduledJobsProject()

	assert.ErrorContains(t, tested.Create(t.Context(), project, api.CreateOptions{}), scheduledJobsRefused)
	assert.ErrorContains(t, tested.Up(t.Context(), project, api.UpOptions{}), scheduledJobsRefused)
	assert.ErrorContains(t, tested.Start(t.Context(), project.Name, api.StartOptions{Project: project}), scheduledJobsRefused)
}

// RunJob reports the missing engine support with a sentinel before doing any
// work, so the CLI can fall back to running the job like a one-off service.
func TestRunJobWithoutJobsEngine(t *testing.T) {
	project := scheduledJobsProject()
	_, err := newJobsTestService(t, noJobsEngine).RunJob(t.Context(), project, "migrate", api.RunOptions{Service: "migrate"})
	assert.ErrorIs(t, err, api.ErrJobsNotSupported)
}

// Down only talks to the engine's jobs registry when the engine has one.
func TestDownJobsWithoutJobsEngine(t *testing.T) {
	project := scheduledJobsProject()

	t.Run("without the jobs API there is nothing to remove", func(t *testing.T) {
		tested := newJobsTestService(t, noJobsEngine)
		assert.Equal(t, len(tested.ensureJobsDown(t.Context(), project)), 0)
	})

	t.Run("with the jobs API every declared job is removed", func(t *testing.T) {
		tested := newJobsTestService(t, apiVersionJobs)
		assert.Equal(t, len(tested.ensureJobsDown(t.Context(), project)), len(project.Jobs))
	})
}
