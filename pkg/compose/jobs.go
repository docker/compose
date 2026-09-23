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
	"io"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	extensionclient "github.com/moby/extensions/client"
	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

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
	case job.Triggers.Manual != nil && *job.Triggers.Manual && len(job.Triggers.Schedule) > 0:
		return nil, fmt.Errorf("job %q declares both manual:true and a schedule, exactly one is supported", job.Name)
	case job.Triggers.Manual != nil && *job.Triggers.Manual:
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
	// like any other service container. svc.CustomLabels is trustworthy here
	// because JobAsService is the single place both materialization paths
	// (RunJob's cmd-layer materializeManualJob and registerScheduledJobs'
	// scopedProjectForJob) build it — the same job's spec no longer differs
	// depending on how it was triggered.
	cfgs, err := s.getCreateConfigs(ctx, project, svc, 1, nil, createOptions{
		UseNetworkAliases: useNetworkAliases,
		Labels:            mergeLabels(svc.Labels, svc.CustomLabels),
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
	if keep == nil {
		return sortedMapKeys(jobs)
	}
	filtered := make(types.Jobs, len(jobs))
	for name, job := range jobs {
		if keep(job) {
			filtered[name] = job
		}
	}
	return sortedMapKeys(filtered)
}

// jobChangedErr reports that the engine already has a job of this name with
// a different spec: re-running the same verb won't reconcile it, only
// `down` removing it first will.
func jobChangedErr(name, verb string) error {
	return fmt.Errorf("job %q has changed: run `docker compose down` to remove it, then `%s` again", name, verb)
}

// mapAlreadyExists maps err through jobsv0.MapError, translating an
// AlreadyExists (the engine already has name registered with a different
// spec) into jobChangedErr instead of the raw engine error.
func mapAlreadyExists(err error, name, verb string) error {
	err = jobsv0.MapError(err)
	if errdefs.IsAlreadyExists(err) {
		return jobChangedErr(name, verb)
	}
	return err
}

// HasSchedule reports whether a job declares a schedule trigger — the
// predicate `up` uses to register it with the engine instead of warning
// that it waits for `docker compose run`.
func HasSchedule(job types.JobConfig) bool {
	return job.Triggers != nil && len(job.Triggers.Schedule) > 0
}

// ManualTriggerDisabled reports whether a job explicitly opts out of manual
// triggering with `triggers.manual: false`. Per the spec, every job accepts
// `docker compose run` regardless of its automated triggers unless it opts
// out this way — this is the single source of truth for that rule, shared
// by every layer that materializes or runs a job manually.
func ManualTriggerDisabled(job types.JobConfig) bool {
	return job.Triggers != nil && job.Triggers.Manual != nil && !*job.Triggers.Manual
}

// ManualTriggerDisabledErr reports that name was declared with
// `manual: false` and so cannot be run manually — the error every caller
// of ManualTriggerDisabled raises on that condition.
func ManualTriggerDisabledErr(name string) error {
	return fmt.Errorf("job %q is declared with manual: false, it cannot be run manually", name)
}

// JobAsService materializes a job as a service for the one-off machinery: a
// job is a ContainerSpec+WorkloadSpec, the same layers a service is made of.
// It carries the standard custom labels the loader stamps on every service —
// materialization happens after loading, so without them the containers
// created for the job would be invisible to every label-driven path: start
// would silently skip them, ps/down would not see them, and the dependency
// wait would report the job as a missing dependency. Exported as the single
// source of truth both materialization paths (cmd/compose/run.go's
// materializeManualJob for a manual run, scopedProjectForJob below for a
// scheduled registration) share, so the same job's spec doesn't differ
// depending on how it was triggered.
func JobAsService(project *types.Project, name string, job types.JobConfig) types.ServiceConfig {
	svc := types.ServiceConfig{
		Name:          name,
		Profiles:      job.Profiles,
		Extensions:    job.Extensions,
		ContainerSpec: job.ContainerSpec,
		WorkloadSpec:  job.WorkloadSpec,
	}
	svc.CustomLabels = types.Labels{
		api.ProjectLabel:     project.Name,
		api.ServiceLabel:     name,
		api.VersionLabel:     api.ComposeVersion,
		api.WorkingDirLabel:  project.WorkingDir,
		api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
		api.OneoffLabel:      "False",
	}
	return svc
}

// scopedProjectForJob returns a shallow copy of project with name's job
// materialized into a fresh Services map, so a scheduled job can run
// through the same project-wide normalization passes (useAPISocket,
// ensureImagesExists, ensureModels) that a manually-run job gets for free
// once materializeManualJob puts it into the real project.Services. The
// copy is throwaway: callers must discard it once they've read back the
// resolved ServiceConfig, so the job never joins the project the real
// service-reconciliation loop (create/start) iterates over.
//
// Configs, and each service's Environment/CustomLabels/Volumes/Configs,
// also get their own copy: registerScheduledJobs calls this concurrently
// for every scheduled job, and the normalization passes above mutate those
// fields in place across every service in the project, not just the job
// being registered — useAPISocket writes project.Configs["#apisocket"],
// service.Environment["DOCKER_CONFIG"], and appends to service.Volumes and
// service.Configs; ensureModels' SetModelVariables writes
// service.Environment for any service with a models: reference; and
// ensureImagesExists writes service.CustomLabels (via Labels.Add, which
// mutates its receiver) and, through resolveImageVolumes, service.Volumes
// elements in place for any `type: image` volume. A plain struct copy of
// ServiceConfig leaves all of these aliased to the real project's, so two
// jobs registering concurrently would both write into the same map or
// backing array — a data race.
func scopedProjectForJob(project *types.Project, name string, job types.JobConfig) *types.Project {
	scoped := *project
	services := make(types.Services, len(project.Services)+1)
	for n, svc := range project.Services {
		env := make(types.MappingWithEquals, len(svc.Environment))
		for k, v := range svc.Environment {
			env[k] = v
		}
		svc.Environment = env

		labels := make(types.Labels, len(svc.CustomLabels))
		for k, v := range svc.CustomLabels {
			labels[k] = v
		}
		svc.CustomLabels = labels

		svc.Volumes = append([]types.ServiceVolumeConfig{}, svc.Volumes...)
		svc.Configs = append([]types.ServiceConfigObjConfig{}, svc.Configs...)

		services[n] = svc
	}
	services[name] = JobAsService(project, name, job)
	scoped.Services = services

	configs := make(types.Configs, len(project.Configs))
	for n, cfg := range project.Configs {
		configs[n] = cfg
	}
	scoped.Configs = configs
	return &scoped
}

// registerScheduledJobs registers the project's scheduled jobs with the
// engine. Create is idempotent on SpecHash: re-applying the same spec on a
// later `up` is a no-op, which is what makes `up` safely re-runnable.
func (s *composeService) registerScheduledJobs(ctx context.Context, project *types.Project) error {
	names := sortedJobNames(project.Jobs, HasSchedule)
	if len(names) == 0 {
		return nil
	}

	jc, err := s.jobsClient()
	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(s.maxConcurrency)
	for _, name := range names {
		eg.Go(func() error {
			job := project.Jobs[name]
			scoped, err := s.useAPISocket(scopedProjectForJob(project, name, job))
			if err != nil {
				return err
			}
			// Without this, a scheduled job declaring only `build:` (no
			// `image:`) registers successfully here and then fails, every
			// time its schedule fires, because the image was never built —
			// silently, since up's own auto-build only ever scoped to
			// project.ServiceNames() (see pkg/compose/build.go).
			if err := s.ensureImagesExists(ctx, scoped, &api.BuildOptions{Services: []string{name}}, false); err != nil {
				return err
			}
			if err := s.ensureModels(ctx, scoped, false); err != nil {
				return err
			}
			svc, err := scoped.GetService(name)
			if err != nil {
				return err
			}
			spec, err := s.buildJobSpec(ctx, scoped, svc, job, false)
			if err != nil {
				return err
			}
			_, err = jc.Create(ctx, &jobsv0.CreateRequest{
				Name: engineJobName(project, name),
				Spec: spec,
			})
			return mapAlreadyExists(err, name, "up")
		})
	}
	return eg.Wait()
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
	if ManualTriggerDisabled(job) {
		return 0, ManualTriggerDisabledErr(name)
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
	// services the job depends on. RunJob is always called with name ==
	// options.Service, so this is the same scoping prepareRun uses.
	buildOpts := prepareBuildOptions(options)
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
	// A job has no run-assigned container name: the jobs API owns the run
	// container's identity itself. But two independent layers inject
	// CLI/terminal-context defaults into the service before RunJob ever
	// sees it — prepareRun's own Tty/StdinOpen overrides don't apply here,
	// yet cmd/compose/run.go's runOptions.apply still sets
	// target.Tty/StdinOpen from the terminal before materializing the job.
	// Left in place, that leaks into the spec sent to the engine and
	// spuriously conflicts (SpecHash mismatch) with the identical spec `up`
	// already registered for a scheduled job. Restore the job's own
	// declared values (set correctly by JobAsService for both
	// materialization paths) rather than the terminal's.
	svc.Tty = job.Tty
	svc.StdinOpen = job.StdinOpen
	svc.ContainerName = ""

	observed, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return 0, err
	}
	if err := s.waitDependencies(ctx, project, name, svc.DependsOn, observed, 0); err != nil {
		return 0, err
	}
	// A job may reference a sibling service or job — volumes_from, or
	// service:-scoped network_mode/ipc/pid — exactly like a service run
	// would; the daemon knows nothing about compose service names, so these
	// must resolve to live container IDs before the spec reaches it.
	if err := s.resolveRunServiceReferences(ctx, project.Name, &svc); err != nil {
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
	created, err := s.createJobRun(ctx, jc, project, name, job, spec)
	if err != nil {
		return 0, err
	}

	running, err := jc.Wait(ctx, &jobsv0.WaitRequest{
		JobRef:    created.JobID,
		RunRef:    created.ID,
		Condition: jobsv0.WaitConditionRunning,
	})
	if err := jobsv0.MapError(err); err != nil {
		return 0, err
	}
	containerID := created.ContainerID
	if running.Run != nil {
		containerID = running.Run.ContainerID
	}

	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		if containerID == "" {
			return
		}
		if err := s.streamJobLogs(ctx, containerID, svc.Tty); err != nil && ctx.Err() == nil {
			logrus.Debugf("job %q: log stream ended: %v", name, err)
		}
	}()

	waited, err := jc.Wait(ctx, &jobsv0.WaitRequest{
		JobRef: created.JobID,
		RunRef: created.ID,
	})
	<-logsDone
	if err := jobsv0.MapError(err); err != nil {
		return 0, err
	}

	run := waited.Run
	if run == nil {
		return 1, fmt.Errorf("job %q: Wait returned no run", name)
	}
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

// createJobRun starts name's Run on the engine, routed on the job's
// declared trigger rather than on how compose happens to invoke it: the
// engine's CreateAndRun refuses a schedule-trigger spec outright
// ("create-and-run serves manual jobs only"), because registering a cron
// must never imply an immediate run. A manual-trigger job (the opt-out
// default included) is still created and run atomically via CreateAndRun.
// A scheduled job is instead Created — idempotent on SpecHash, a no-op if
// `up` already registered the identical spec, arming the schedule if not —
// then explicitly Run with Reschedule: false, so the manual fire adds to
// the cron cadence instead of replacing its next occurrence.
func (s *composeService) createJobRun(ctx context.Context, jc jobsv0.Jobs, project *types.Project, name string, job types.JobConfig, spec *jobsv0.JobSpec) (*jobsv0.Run, error) {
	engineName := engineJobName(project, name)
	if !HasSchedule(job) {
		reply, err := jc.CreateAndRun(ctx, &jobsv0.CreateAndRunRequest{
			Name: engineName,
			Spec: spec,
		})
		if err := mapAlreadyExists(err, name, "run"); err != nil {
			return nil, err
		}
		if reply.Run == nil {
			return nil, fmt.Errorf("job %q: engine returned no run", name)
		}
		return reply.Run, nil
	}

	_, err := jc.Create(ctx, &jobsv0.CreateRequest{Name: engineName, Spec: spec})
	if err := mapAlreadyExists(err, name, "run"); err != nil {
		return nil, err
	}
	reply, err := jc.Run(ctx, &jobsv0.RunRequest{JobRef: engineName, Reschedule: false})
	if err := jobsv0.MapError(err); err != nil {
		return nil, err
	}
	if reply.Run == nil {
		return nil, fmt.Errorf("job %q: engine returned no run", name)
	}
	return reply.Run, nil
}

// streamJobLogs follows a job Run's container logs from the start, exactly
// like `docker logs -f`, until the container stops producing output. tty
// must match the container's own Tty setting: with a tty allocated, the
// daemon returns a single raw stream instead of the stdout/stderr-framed
// stream stdcopy expects (see doLogContainer in logs.go for the same split).
func (s *composeService) streamJobLogs(ctx context.Context, containerID string, tty bool) error {
	r, err := s.apiClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck
	if tty {
		_, err = io.Copy(s.stdout(), r)
	} else {
		_, err = stdcopy.StdCopy(s.stdout(), s.stderr(), r)
	}
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
