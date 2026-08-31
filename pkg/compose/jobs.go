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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	extensionclient "github.com/moby/extensions/client"
	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	jobsv0 "github.com/docker/compose/v5/internal/jobsapi"
	jobspb "github.com/docker/compose/v5/internal/jobsapi/protogen"
	"github.com/docker/compose/v5/pkg/api"
)

// jobRunHistoryLimit caps retained terminal run records (see
// jobsv0.JobSpec.RunHistoryLimit): low enough that repeated failures of a
// frequently-firing schedule don't accumulate kept containers indefinitely.
const jobRunHistoryLimit = 5

// jobsClient lazily resolves the engine's jobs extension point over the same
// dialer the Docker API client already uses. Resolve never dials by itself —
// an engine without the jobs feature only surfaces codes.Unimplemented on
// the first real call — so projects with no jobs never touch the network.
func (s *composeService) jobsClient() (jobsv0.Jobs, error) {
	s.jobsGRPCOnce.Do(func() {
		exts, err := extensionclient.New(s.apiClient(), extensionclient.WithGRPCPoint(jobspb.ClientPoint))
		if err != nil {
			s.jobsAPIErr = err
			return
		}
		s.jobsExtClient = exts
		s.jobsAPI, s.jobsAPIErr = extensionclient.Resolve(exts, jobsv0.Point)
	})
	return s.jobsAPI, s.jobsAPIErr
}

// engineJobNameSeparator joins a project and job name into the engine's
// daemon-wide job name. The engine only accepts [a-zA-Z0-9][a-zA-Z0-9_.-]* for
// job names, which rules out "/" as a separator (confirmed against a live
// engine: it rejects it with InvalidArgument). "." still works: project names
// are restricted to [a-z0-9_-] (see compose-go's NormalizeProjectName, no
// dot), while job names may contain one, so a "." can only originate from the
// job-name side — the join stays unambiguous to reverse. Unlike api.Separator
// ("-"), which both project and job names can contain, this avoids project
// "app-sub" + job "service" colliding with project "app" + job "sub-service".
const engineJobNameSeparator = "."

// engineJobName is the daemon-wide unique name Compose registers a job
// under: the engine has no notion of Compose projects, so the project name
// is folded into the job name.
func engineJobName(project *types.Project, name string) string {
	return project.Name + engineJobNameSeparator + name
}

// jobTrigger translates compose-go's TriggerConfig into the engine's
// Trigger type: 1:1 field mapping, already verified against the engine
// contract.
func jobTrigger(job types.JobConfig) (*jobsv0.Trigger, error) {
	switch {
	case job.Triggers == nil:
		return nil, fmt.Errorf("job %q has no trigger", job.Name)
	case job.Triggers.Manual:
		return &jobsv0.Trigger{Manual: true}, nil
	case len(job.Triggers.Schedule) == 1:
		sc := job.Triggers.Schedule[0]
		return &jobsv0.Trigger{Schedule: &jobsv0.ScheduleTrigger{
			Cron:        sc.Cron,
			Timezone:    sc.Timezone,
			Concurrency: sc.Concurrency,
			MissedFires: sc.MissedFires,
		}}, nil
	case len(job.Triggers.Schedule) > 1:
		return nil, fmt.Errorf("job %q declares %d schedules, exactly one is supported", job.Name, len(job.Triggers.Schedule))
	default:
		return nil, fmt.Errorf("job %q has no trigger", job.Name)
	}
}

// buildJobSpec builds the engine JobSpec for a job: svc is resolved exactly
// like a service (materializeManualJob or the synthetic ServiceConfig built
// from the JobConfig for scheduled jobs), and getCreateConfigs is the same
// service->container-create-body conversion used to create real containers.
func (s *composeService) buildJobSpec(ctx context.Context, project *types.Project, svc types.ServiceConfig, job types.JobConfig, useNetworkAliases bool) (*jobsv0.JobSpec, error) {
	trigger, err := jobTrigger(job)
	if err != nil {
		return nil, err
	}

	// The engine only carries JobSpec.Labels on the job object, not on the run
	// container (see jobsv0.JobSpec.Labels godoc): project/service labels must
	// be set here on the container spec itself so run containers stay visible
	// to the rest of Compose's tooling (ps, label-scoped listings) exactly
	// like any other service container.
	cfgs, err := s.getCreateConfigs(ctx, project, svc, 1, nil, createOptions{
		UseNetworkAliases: useNetworkAliases,
		Labels: mergeLabels(svc.Labels, svc.CustomLabels, types.Labels{
			api.ProjectLabel: project.Name,
			api.ServiceLabel: job.Name,
		}),
	})
	if err != nil {
		return nil, err
	}
	spec, err := json.Marshal(containerType.CreateRequest{
		Config:           cfgs.Container,
		HostConfig:       cfgs.Host,
		NetworkingConfig: cfgs.Network,
	})
	if err != nil {
		return nil, err
	}

	return &jobsv0.JobSpec{
		ContainerSpec: spec,
		Trigger:       trigger,
		Labels: map[string]string{
			api.ProjectLabel: project.Name,
			api.JobLabel:     job.Name,
		},
		// Successful runs are disposable: drop the container once its
		// terminal record is written. Failed ones are kept for postmortem,
		// bounded by RunHistoryLimit so repeated failures don't accumulate.
		RemoveOnSuccess: true,
		RemoveOnFailure: false,
		RunHistoryLimit: jobRunHistoryLimit,
	}, nil
}

// sortedJobNames returns the sorted names of a project's jobs, optionally
// restricted to those matching keep.
func sortedJobNames(jobs types.Jobs, keep func(types.JobConfig) bool) []string {
	names := make([]string, 0, len(jobs))
	for name, job := range jobs {
		if keep == nil || keep(job) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func hasSchedule(job types.JobConfig) bool {
	return job.Triggers != nil && len(job.Triggers.Schedule) > 0
}

// registerScheduledJobs registers the project's scheduled jobs with the
// engine. Create is idempotent on SpecHash: re-applying the same spec on a
// later `up` is a no-op, which is what makes `up` safely re-runnable.
//
// Known gap: unlike RunJob, a scheduled job never joins project.Services, so
// it isn't seen by the project-wide use_api_socket / models: normalization
// passes (pkg/compose/apiSocket.go, pkg/compose/model.go) — a scheduled job
// declaring either is silently missing that setup. Fixing this without
// risking those services being picked up by the real service-reconciliation
// loop needs more than this diff's scope.
func (s *composeService) registerScheduledJobs(ctx context.Context, project *types.Project) error {
	names := sortedJobNames(project.Jobs, hasSchedule)
	if len(names) == 0 {
		return nil
	}

	jc, err := s.jobsClient()
	if err != nil {
		return err
	}
	for _, name := range names {
		job := project.Jobs[name]
		svc := types.ServiceConfig{
			Name:          name,
			Profiles:      job.Profiles,
			ContainerSpec: job.ContainerSpec,
			WorkloadSpec:  job.WorkloadSpec,
		}
		spec, err := s.buildJobSpec(ctx, project, svc, job, false)
		if err != nil {
			return err
		}
		_, err = jc.Create(ctx, &jobsv0.CreateRequest{
			Name: engineJobName(project, name),
			Spec: spec,
		})
		err = jobsv0.MapError(err)
		if errdefs.IsAlreadyExists(err) {
			return fmt.Errorf("job %q has changed: run `docker compose down` to remove it, then `up` again", name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// RunJob triggers a manual-trigger job's Run on the engine, following the
// same dependency-startup path as a one-off service run, then streams the
// Run's container logs and waits for its terminal state. See the RunJob
// doc comment on api.Compose for which options fields have an effect.
func (s *composeService) RunJob(ctx context.Context, project *types.Project, name string, options api.RunOptions) (int, error) {
	job, ok := project.AllJobs()[name]
	if !ok {
		return 0, fmt.Errorf("job %q not found", name)
	}
	if job.Triggers == nil || !job.Triggers.Manual {
		return 0, fmt.Errorf("job %q has no manual trigger, it cannot be run", name)
	}

	// materializeManualJob already put the job into project.Services, so it
	// is seen by these project-wide normalization passes exactly like a
	// service would be (use_api_socket / models: support).
	project, err := s.useAPISocket(project)
	if err != nil {
		return 0, err
	}

	if err := s.startDependencies(ctx, project, api.RunOptions{
		Service:       name,
		NoDeps:        options.NoDeps,
		CreateOptions: options.CreateOptions,
	}); err != nil {
		return 0, err
	}
	// The job's own image build (if any) is scoped to just this job, unlike
	// startDependencies' unscoped Build above which may also build other
	// services the job depends on.
	var buildOpts *api.BuildOptions
	if options.Build != nil {
		bo := *options.Build
		bo.Services = []string{name}
		buildOpts = &bo
	}
	if err := s.ensureImagesExists(ctx, project, buildOpts, options.QuietPull); err != nil {
		return 0, err
	}
	if err := s.ensureModels(ctx, project, false); err != nil {
		return 0, err
	}

	svc, err := project.GetService(name)
	if err != nil {
		return 0, err
	}
	applyRunOptions(project, &svc, options)
	project.Services[name] = svc

	observed, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return 0, err
	}
	if err := s.waitDependencies(ctx, project, name, svc.DependsOn, observed, 0); err != nil {
		return 0, err
	}
	spec, err := s.buildJobSpec(ctx, project, svc, job, options.UseNetworkAliases)
	if err != nil {
		return 0, err
	}

	jc, err := s.jobsClient()
	if err != nil {
		return 0, err
	}
	reply, err := jc.CreateAndRun(ctx, &jobsv0.CreateAndRunRequest{
		Name: engineJobName(project, name),
		Spec: spec,
	})
	if err := jobsv0.MapError(err); err != nil {
		if errdefs.IsAlreadyExists(err) {
			return 0, fmt.Errorf("job %q has changed: run `docker compose down` to remove it, then `run` again", name)
		}
		return 0, err
	}

	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		if err := s.streamJobLogs(ctx, reply.Run.ContainerID); err != nil && ctx.Err() == nil {
			logrus.Debugf("job %q: log stream ended: %v", name, err)
		}
	}()

	waited, err := jc.Wait(ctx, &jobsv0.WaitRequest{
		JobRef: reply.Job.ID,
		RunRef: reply.Run.ID,
	})
	<-logsDone
	if err := jobsv0.MapError(err); err != nil {
		return 0, err
	}

	run := waited.Run
	switch run.State {
	case jobsv0.RunStateSucceeded:
		return 0, nil
	case jobsv0.RunStateFailed:
		if run.ExitCode != nil {
			return int(run.ExitCode.Value), nil
		}
		return 1, fmt.Errorf("job %q run %s failed: %s", name, run.ID, run.Error)
	case jobsv0.RunStateTimedOut:
		return 124, nil
	case jobsv0.RunStateCancelled:
		return 130, nil
	default:
		return 1, fmt.Errorf("job %q run %s ended in unexpected state %q", name, run.ID, run.State)
	}
}

// streamJobLogs follows a job Run's container logs from the start, exactly
// like `docker logs -f`, until the container stops producing output.
func (s *composeService) streamJobLogs(ctx context.Context, containerID string) error {
	r, err := s.apiClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck
	_, err = stdcopy.StdCopy(s.stdout(), s.stderr(), r)
	return err
}

// ensureJobsDown removes the project's jobs from the engine. Run history is
// kept by default (RunsRemoval left empty); each removal tolerates the job
// already being gone, matching removeResource's NotFound handling.
func (s *composeService) ensureJobsDown(ctx context.Context, project *types.Project) []downOp {
	names := sortedJobNames(project.Jobs, nil)

	var ops []downOp
	for _, name := range names {
		ops = append(ops, func() error {
			return s.removeResource("Job "+name, func() error {
				jc, err := s.jobsClient()
				if err != nil {
					return err
				}
				err = jc.Remove(ctx, &jobsv0.RemoveRequest{JobRef: engineJobName(project, name)})
				return jobsv0.MapError(err)
			})
		})
	}
	return ops
}

// actualJobs reconstructs a project's jobs from the engine's own registry,
// keyed by their local (un-prefixed) name — used when down has no compose
// file to read Jobs from directly (e.g. `compose --project-name X down`).
// Best-effort: any error, including an engine with no jobs feature, is
// treated as "no jobs" so it never blocks an otherwise-successful down.
func (s *composeService) actualJobs(ctx context.Context, projectName string) types.Jobs {
	jc, err := s.jobsClient()
	if err != nil {
		return nil
	}
	reply, err := jc.List(ctx, &jobsv0.ListRequest{
		Labels: []string{api.ProjectLabel + "=" + projectName},
	})
	if err := jobsv0.MapError(err); err != nil {
		// Best-effort: down must not fail or get noisy just because this
		// project has no jobs (or the engine has no jobs feature at all —
		// which surfaces as anything from a clean Unimplemented to a raw
		// transport error, depending on the daemon). Still traceable with
		// -v for the case where jobs really were left behind.
		logrus.Debugf("failed to list jobs for project %q: %v", projectName, err)
		return nil
	}

	prefix := projectName + engineJobNameSeparator
	jobs := types.Jobs{}
	for _, j := range reply.Jobs {
		name := strings.TrimPrefix(j.Name, prefix)
		jobs[name] = types.JobConfig{Name: name}
	}
	return jobs
}
