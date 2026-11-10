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
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	extensionclient "github.com/moby/extensions/client"
	"github.com/moby/moby/api/pkg/stdcopy"
	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	jobsv0 "github.com/docker/compose/v5/internal/jobsapi"
	jobspb "github.com/docker/compose/v5/internal/jobsapi/protogen"
	"github.com/docker/compose/v5/internal/naming"
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

// jobsStatus is what the engine offers for jobs.
type jobsStatus int

const (
	jobsAvailable jobsStatus = iota
	jobsEngineTooOld
	jobsExtensionOff
)

// errJobsExtensionNotReady reports a jobs extension that is enabled but does
// not answer yet (or failed to activate): not missing support, a failure.
var errJobsExtensionNotReady = errors.New("the engine's jobs extension is enabled but not ready")

// jobsAvailability reports what the engine offers for jobs. The API version
// only says the extension is compiled in: it is disabled by default and the
// engine cannot list its extensions, so a call scoped to the project tells
// (Unimplemented: off). Unavailable (enabled, not ready) is an error, never a
// status, so no caller degrades on it.
func (s *composeService) jobsAvailability(ctx context.Context, projectName string) (jobsStatus, error) {
	version, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return jobsAvailable, err
	}
	if versions.LessThan(version, apiVersionJobs) {
		return jobsEngineTooOld, nil
	}

	s.jobsProbe.mu.Lock()
	defer s.jobsProbe.mu.Unlock()
	if s.jobsProbe.known {
		return s.jobsProbe.status, nil
	}
	probed, err := s.probeJobs(ctx, projectName)
	if err != nil {
		return jobsAvailable, err
	}
	s.jobsProbe.status, s.jobsProbe.known = probed, true
	return probed, nil
}

func (s *composeService) probeJobs(ctx context.Context, projectName string) (jobsStatus, error) {
	jc, err := s.jobsClient()
	if err != nil {
		return jobsAvailable, err
	}
	_, err = jc.List(ctx, &jobsv0.ListRequest{Labels: []string{naming.LabelFilter(api.ProjectLabel, projectName)}})
	switch status.Code(err) {
	case codes.OK:
		return jobsAvailable, nil
	case codes.Unimplemented:
		return jobsExtensionOff, nil
	case codes.Unavailable:
		return jobsAvailable, fmt.Errorf("%w: %s", errJobsExtensionNotReady, status.Convert(err).Message())
	default:
		return jobsAvailable, err
	}
}

// rejectUnsupportedScheduledJobs refuses a project declaring active scheduled
// jobs when the engine cannot run them: silently not scheduling them would
// break the user's expectations, unlike manual jobs which simply wait for an
// explicit trigger.
func (s *composeService) rejectUnsupportedScheduledJobs(ctx context.Context, project *types.Project) error {
	names := sortedJobNames(project.Jobs, HasSchedule)
	if len(names) == 0 {
		return nil
	}
	st, err := s.jobsAvailability(ctx, project.Name)
	if err != nil {
		return err
	}
	switch st {
	case jobsEngineTooOld:
		return fmt.Errorf("scheduled jobs are not supported by this version of the engine: %s", strings.Join(names, ", "))
	case jobsExtensionOff:
		return fmt.Errorf("scheduled jobs need the engine's jobs extension, which is not enabled (start dockerd with %s): %s",
			jobsv0.EnableFlag, strings.Join(names, ", "))
	}
	return nil
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
	//
	// The engine hashes the container spec, labels included, and refuses a job
	// whose spec changed: ensureImagesExists' image digest label would make up
	// and run fail each time the image behind the same reference is pulled
	// again, though the engine creates every run from that reference and uses
	// the new image on its own.
	labels := mergeLabels(svc.Labels, svc.CustomLabels)
	delete(labels, api.ImageDigestLabel)
	cfgs, err := s.getCreateConfigs(ctx, project, svc, 1, nil, createOptions{
		UseNetworkAliases: useNetworkAliases,
		Labels:            labels,
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
		// A scheduled job's successful runs are disposable: the engine drops
		// the container once its terminal record is written. A manual job's
		// are kept, logs included, until RunJob has read them and removed the
		// container itself: it is never registered by up, so its spec is free
		// to differ, unlike a scheduled job's, which `run` must send exactly as
		// up registered it. Failed runs are kept for postmortem, bounded by
		// RunHistoryLimit so repeated failures don't accumulate.
		RemoveOnSuccess: HasSchedule(job),
		RemoveOnFailure: false,
		RunHistoryLimit: jobRunHistoryLimit,
	}, nil
}

// sortedJobNames returns the sorted names of a project's jobs, optionally
// restricted to those matching keep.
func sortedJobNames(jobs types.Jobs, keep func(types.JobConfig) bool) []string {
	if keep == nil {
		return slices.Sorted(maps.Keys(jobs))
	}
	filtered := make(types.Jobs, len(jobs))
	for name, job := range jobs {
		if keep(job) {
			filtered[name] = job
		}
	}
	return slices.Sorted(maps.Keys(filtered))
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
		api.WorkingDirLabel:  project.WorkingDir,
		api.ConfigFilesLabel: strings.Join(project.ComposeFiles, ","),
		api.OneoffLabel:      "False",
	}
	return svc
}

// scopedProjectForJob returns a shallow copy of project with name's job
// materialized as its only service, so a scheduled job can run through the same
// project-wide normalization passes (useAPISocket, ensureImagesExists,
// ensureModels) that a manually-run job gets for free once materializeManualJob
// puts it into the real project.Services. The services stay in the copy, as
// disabled ones, for the job to name them (see resolveJobServiceReferences): the
// passes then leave their images alone, `up` already went through them. The
// copy is throwaway: callers discard it once they've read back the resolved
// ServiceConfig, so the job never joins the project the reconciliation loop
// iterates over.
//
// Those passes write into the job's Environment, Volumes and Configs, and into
// the project's Configs, in place: the job and the project's Configs get their
// own copy, so nothing is written through to project.Jobs or to the Configs the
// other jobs registering concurrently copy from.
func scopedProjectForJob(project *types.Project, name string, job types.JobConfig) *types.Project {
	scoped := *project
	scoped.Services = types.Services{name: withOwnMutableFields(JobAsService(project, name, job))}
	scoped.DisabledServices = make(types.Services, len(project.Services)+len(project.DisabledServices))
	maps.Copy(scoped.DisabledServices, project.DisabledServices)
	maps.Copy(scoped.DisabledServices, project.Services)
	scoped.Configs = maps.Clone(project.Configs)
	return &scoped
}

// withOwnMutableFields returns svc with its own copy of the fields the
// normalization passes write into in place. CustomLabels is not one of them:
// JobAsService builds it afresh.
func withOwnMutableFields(svc types.ServiceConfig) types.ServiceConfig {
	// not nil: SetModelVariables and useAPISocket write into it
	env := make(types.MappingWithEquals, len(svc.Environment))
	maps.Copy(env, svc.Environment)
	svc.Environment = env
	svc.Volumes = slices.Clone(svc.Volumes)
	svc.Configs = slices.Clone(svc.Configs)
	return svc
}

// resolveJobServiceReferences replaces the services a job shares volumes or
// namespaces with by the name of their first container. The engine knows no
// Compose service, and an ID would not survive the service being recreated: it
// would change the spec, which the engine hashes and holds immutable, and the
// container a cron fires on later. Both `up` and `run` go through here, so the
// spec of a job is the same whichever of them sends it.
func resolveJobServiceReferences(project *types.Project, service *types.ServiceConfig) error {
	return resolveReferences(service, func(name string) (string, bool) {
		target, ok := project.Services[name]
		if !ok {
			// the selection (--no-deps, up <services>) narrows the project
			target, ok = project.DisabledServices[name]
		}
		if !ok {
			return "", false
		}
		return naming.ContainerName(project.Name, target, 1), true
	})
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
			if err := s.ensureImagesExists(ctx, scoped, &api.BuildOptions{Services: []string{name}}, false, false); err != nil {
				return err
			}
			if err := s.ensureModels(ctx, scoped, false); err != nil {
				return err
			}
			svc, err := scoped.GetService(name)
			if err != nil {
				return err
			}
			if err := resolveJobServiceReferences(scoped, &svc); err != nil {
				return err
			}
			spec, err := s.buildJobSpec(ctx, scoped, svc, job, false)
			if err != nil {
				return err
			}
			if s.dryRun {
				return nil // the jobs gRPC dials the real engine, bypassing the dry-run client
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
	st, err := s.jobsAvailability(ctx, project.Name)
	if err != nil {
		return 0, err
	}
	switch st {
	case jobsEngineTooOld:
		version, _ := s.RuntimeAPIVersion(ctx) // memoized by jobsAvailability
		return 0, fmt.Errorf("%w: this version of the engine (API version %s) predates jobs", api.ErrJobsNotSupported, version)
	case jobsExtensionOff:
		return 0, fmt.Errorf("%w: the jobs extension is not enabled (start dockerd with %s)", api.ErrJobsNotSupported, jobsv0.EnableFlag)
	}
	// Past the availability switch so a degraded run, a one-off service, keeps
	// honouring -d and dry-run.
	if options.Detach {
		return 0, errors.New("--detach is not supported for a job: it runs until it is done")
	}
	if err := refuseScheduledJobOverrides(project, name, job, options); err != nil {
		return 0, err
	}
	if s.dryRun {
		return 0, nil // the jobs gRPC dials the real engine, see registerScheduledJobs
	}
	if options.Name != "" {
		logrus.Warnf("--name has no effect on a job: the engine names its run containers itself")
	}

	// materializeManualJob already put the job into project.Services, so it
	// is seen by these project-wide normalization passes exactly like a
	// service would be (use_api_socket / models: support).
	project, err = s.useAPISocket(project)
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
	if err := s.ensureImagesExists(ctx, project, buildOpts, options.QuietPull, false); err != nil {
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
	// makes it differ from one run to the next (SpecHash mismatch). Restore
	// the job's own declared values (set correctly by JobAsService for both
	// materialization paths) rather than the terminal's.
	svc.Tty = job.Tty
	svc.StdinOpen = job.StdinOpen
	svc.ContainerName = ""

	observed, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return 0, err
	}
	if err := s.waitDependencies(ctx, project, name, svc.DependsOn, observed); err != nil {
		return 0, err
	}
	// A scheduled job has no spec to send: it runs as `up` registered it.
	var spec *jobsv0.JobSpec
	// The engine kept the container of a manual job (see buildJobSpec): removing
	// it is up to us. An interrupted run leaves it behind, an orphan that
	// `down --remove-orphans` sweeps.
	keepContainer := false
	if !HasSchedule(job) {
		spec, err = s.manualJobSpec(ctx, project, svc, job, options.UseNetworkAliases)
		if err != nil {
			return 0, err
		}
		keepContainer = !spec.RemoveOnSuccess
	}

	jc, err := s.jobsClient()
	if err != nil {
		return 0, err
	}
	created, err := s.createJobRun(ctx, jc, project, name, job, spec)
	if err != nil {
		return 0, err
	}

	// a manual job, which has a spec to send, does not outlive its run
	defer s.endJobRun(ctx, jc, name, created.JobID, spec != nil)

	containerID, err := runContainerID(ctx, jc, created)
	if err != nil {
		return 0, err
	}

	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		if containerID == "" {
			return
		}
		if err := s.streamJobLogs(ctx, containerID, svc.Tty); err != nil && ctx.Err() == nil {
			reportJobLogsError(name, err)
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
	if keepContainer && run.State == jobsv0.RunStateSucceeded {
		// ctx may be done: an interrupted run still has its container to remove
		s.removeJobRunContainer(context.WithoutCancel(ctx), name, containerID)
	}
	return jobRunExit(name, run)
}

// endJobRun is what `run` does once it is over, however it ended: stop the run
// if it was interrupted, and remove a manual job from the engine. The job is
// not kept for the next `run`: the engine holds a job to the spec it was
// registered with, and the next `run` may carry other options. Remove keeps the
// run records and the run containers, the failures left for a postmortem.
func (s *composeService) endJobRun(ctx context.Context, jc jobsv0.Jobs, name, jobID string, manual bool) {
	cancelInterruptedJobRun(ctx, jc, name, jobID)
	if !manual {
		return
	}
	// ctx may be done: an interrupted run still has its job to remove
	if err := jc.Remove(context.WithoutCancel(ctx), &jobsv0.RemoveRequest{JobRef: jobID}); err != nil {
		if err = jobsv0.MapError(err); !errdefs.IsNotFound(err) {
			logrus.Warnf("job %q: could not remove it from the engine: %v", name, err)
		}
	}
}

// cancelInterruptedJobRun stops the run of a `run` that was interrupted: the
// engine owns the job, and cancelling the context only detaches Wait from it.
// ctx is already done then, so the Cancel runs on a context detached from it.
func cancelInterruptedJobRun(ctx context.Context, jc jobsv0.Jobs, name, jobID string) {
	if ctx.Err() == nil {
		return
	}
	if _, err := jc.Cancel(context.WithoutCancel(ctx), &jobsv0.CancelRequest{JobRef: jobID}); err != nil {
		logrus.Warnf("could not cancel job %q, it keeps running on the engine: %v", name, jobsv0.MapError(err))
	}
}

// runContainerID returns the container backing a just-created run, waiting for
// the engine to create it if it has not yet: callers attach to its logs as soon
// as it exists, since a quick job's container may vanish when it succeeds.
func runContainerID(ctx context.Context, jc jobsv0.Jobs, created *jobsv0.Run) (string, error) {
	if created.ContainerID != "" {
		return created.ContainerID, nil
	}
	running, err := jc.Wait(ctx, &jobsv0.WaitRequest{
		JobRef:    created.JobID,
		RunRef:    created.ID,
		Condition: jobsv0.WaitConditionRunning,
	})
	if err := jobsv0.MapError(err); err != nil {
		return "", err
	}
	if running.Run == nil {
		logrus.Warnf("run %s: the engine reported no container, its output cannot be shown", created.ID)
		return "", nil
	}
	return running.Run.ContainerID, nil
}

// jobRunExit translates a terminal run record into the exit code `run` ends
// with, the way a one-off container's exit status would be.
func jobRunExit(name string, run *jobsv0.Run) (int, error) {
	switch run.State {
	case jobsv0.RunStateSucceeded:
		return 0, nil
	case jobsv0.RunStateFailed:
		if run.ExitCode != nil {
			return int(run.ExitCode.Value), nil
		}
		return 1, fmt.Errorf("job %q run %s failed: %s", name, run.ID, run.Error)
	case jobsv0.RunStateTimedOut:
		return 124, fmt.Errorf("job %q timed out", name)
	case jobsv0.RunStateCancelled:
		return 130, fmt.Errorf("job %q was cancelled", name)
	default:
		return 1, fmt.Errorf("job %q run %s ended in unexpected state %q", name, run.ID, run.State)
	}
}

// manualJobSpec builds the spec a manual job is run with.
func (s *composeService) manualJobSpec(ctx context.Context, project *types.Project, svc types.ServiceConfig, job types.JobConfig, useNetworkAliases bool) (*jobsv0.JobSpec, error) {
	// A job may reference a service — volumes_from, or service:-scoped
	// network_mode/ipc/pid — exactly like a service run would; the daemon
	// knows nothing about compose service names, so these must resolve to
	// container names before the spec reaches it.
	if err := resolveJobServiceReferences(project, &svc); err != nil {
		return nil, err
	}
	// --no-deps narrowed svc.DependsOn to what this run waits for: the spec
	// carries the declared ones, the same on every run.
	svc.DependsOn = job.DependsOn
	return s.buildJobSpec(ctx, project, svc, job, useNetworkAliases)
}

// refuseScheduledJobOverrides is an error when run asks to change a scheduled job.
func refuseScheduledJobOverrides(project *types.Project, name string, job types.JobConfig, options api.RunOptions) error {
	svc, ok := project.Services[name]
	if !ok || !HasSchedule(job) {
		return nil
	}
	overrides := scheduledJobOverrides(options, svc, job)
	if len(overrides) == 0 {
		return nil
	}
	return fmt.Errorf("scheduled job %q runs as registered by up: it takes no %s, "+
		"change the compose file and run up again", name, strings.Join(overrides, ", "))
}

// scheduledJobOverrides names what a `run` asks to change in a scheduled job: it
// runs as `up` registered it. The options of the CLI that the cmd layer applies
// to the service itself, -v and -p, show as a difference from the job as declared.
func scheduledJobOverrides(options api.RunOptions, svc types.ServiceConfig, job types.JobConfig) []string {
	var overrides []string
	for _, o := range []struct {
		name string
		set  bool
	}{
		{"a command", len(options.Command) > 0},
		{"--entrypoint", len(options.Entrypoint) > 0},
		{"--user", options.User != ""},
		{"--workdir", options.WorkingDir != ""},
		{"--env/--env-from-file", len(options.Environment) > 0},
		{"--cap-add", len(options.CapAdd) > 0},
		{"--cap-drop", len(options.CapDrop) > 0},
		{"--label", len(options.Labels) > 0},
		{"--volume", !reflect.DeepEqual(svc.Volumes, job.Volumes)},
		{"--publish", !reflect.DeepEqual(svc.Ports, job.Ports)},
	} {
		if o.set {
			overrides = append(overrides, o.name)
		}
	}
	return overrides
}

// createJobRun starts name's Run on the engine, routed on the job's
// declared trigger rather than on how compose happens to invoke it: the
// engine's CreateAndRun refuses a schedule-trigger spec outright
// ("create-and-run serves manual jobs only"), because registering a cron
// must never imply an immediate run. A manual-trigger job (the opt-out
// default included) is still created and run atomically via CreateAndRun.
// A scheduled job is run as `up` registered it, spec is nil: sending one to
// Create would arm the schedule of a job `up` never registered, and a spec
// differing from the registered one is refused. It is Run with Reschedule:
// false, so the manual fire adds to the cron cadence instead of replacing its
// next occurrence.
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

	reply, err := jc.Run(ctx, &jobsv0.RunRequest{JobRef: engineName, Reschedule: false})
	if err := jobsv0.MapError(err); err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("job %q is not registered with the engine: run `docker compose up` (or `create`) first", name)
		}
		return nil, err
	}
	if reply.Run == nil {
		return nil, fmt.Errorf("job %q: engine returned no run", name)
	}
	return reply.Run, nil
}

// reportJobLogsError tells why a job's logs could not be shown in full. A
// container gone before its logs were read is the engine's RemoveOnSuccess at
// work on a job that finished too fast: the run succeeded, its output is lost.
func reportJobLogsError(name string, err error) {
	if errdefs.IsNotFound(err) {
		logrus.Warnf("job %q: its logs are gone, the engine removed the run container before they could be read", name)
		return
	}
	logrus.Debugf("job %q: log stream ended: %v", name, err)
}

// removeJobRunContainer removes the container of a manual job's successful
// run, which RunJob asked the engine to keep so the logs could be read.
func (s *composeService) removeJobRunContainer(ctx context.Context, name, containerID string) {
	if _, err := s.apiClient().ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{RemoveVolumes: true}); err != nil && !errdefs.IsNotFound(err) {
		logrus.Warnf("job %q: failed to remove its run container %s: %v", name, containerID, err)
	}
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
	// a manual job does not outlive its run, unless `run` was killed before it
	// could remove it: every job goes, a missing one is tolerated
	names := sortedJobNames(project.Jobs, nil)
	if len(names) == 0 {
		return nil
	}
	st, err := s.jobsAvailability(ctx, project.Name)
	if errors.Is(err, errJobsExtensionNotReady) {
		logrus.Warnf("jobs of project %q not removed: %v", project.Name, err)
	}
	if err != nil || st != jobsAvailable {
		return nil
	}

	var ops []downOp
	for _, name := range names {
		ops = append(ops, func() error {
			return s.removeResource("Job "+name, func() error {
				if s.dryRun {
					return nil // see registerScheduledJobs
				}
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
// treated as "no jobs" so it never blocks an otherwise-successful down; an
// extension enabled but not ready is the one case worth a warning.
func (s *composeService) actualJobs(ctx context.Context, projectName string) types.Jobs {
	st, err := s.jobsAvailability(ctx, projectName)
	if errors.Is(err, errJobsExtensionNotReady) {
		logrus.Warnf("jobs of project %q not removed: %v", projectName, err)
	}
	if err != nil || st != jobsAvailable {
		return nil
	}
	jc, err := s.jobsClient()
	if err != nil {
		return nil
	}
	reply, err := jc.List(ctx, &jobsv0.ListRequest{
		Labels: []string{naming.LabelFilter(api.ProjectLabel, projectName)},
	})
	if err != nil {
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

// orphanJobs names the jobs the engine has registered for the project that the
// project no longer declares — a job renamed or removed from the file since
// `up` registered it. A job out of the profiles of this run is declared, not
// an orphan, which is what AllJobs is for.
func (s *composeService) orphanJobs(ctx context.Context, project *types.Project) types.Jobs {
	declared := project.AllJobs()
	orphans := types.Jobs{}
	for name, job := range s.actualJobs(ctx, project.Name) {
		if _, ok := declared[name]; !ok {
			orphans[name] = job
		}
	}
	return orphans
}

// loadJobsToRemove completes the jobs down removes with what the engine has
// registered for the project. Without a compose file to read them from they are
// reconstructed from the registry, otherwise a project's scheduled jobs would
// outlive `down` forever. With one, --remove-orphans adds the orphan jobs, the
// ones an earlier up registered from a file that declared more; a partial down
// leaves the jobs alone.
func (s *composeService) loadJobsToRemove(ctx context.Context, project *types.Project, projectName string, options api.DownOptions) {
	switch {
	case options.Project == nil && len(options.Services) == 0:
		project.Jobs = s.actualJobs(ctx, projectName)
	case options.RemoveOrphans && len(options.Services) == 0:
		if project.Jobs == nil {
			project.Jobs = types.Jobs{}
		}
		maps.Copy(project.Jobs, s.orphanJobs(ctx, project))
	}
}

// handleOrphanJobs tells the user about the orphan jobs of the project, which
// keep firing on their schedule, or removes them with --remove-orphans. The
// registry is read when it can matter only: by a project that declares
// scheduled jobs, or one that was asked to clean up.
func (s *composeService) handleOrphanJobs(ctx context.Context, project *types.Project, removeOrphans, ignoreOrphans bool) error {
	if !removeOrphans && len(sortedJobNames(project.Jobs, HasSchedule)) == 0 {
		return nil
	}
	orphans := s.orphanJobs(ctx, project)
	if len(orphans) == 0 {
		return nil
	}
	if !removeOrphans {
		if !ignoreOrphans {
			logrus.Warnf("Found orphan jobs (%s) still registered with the engine for this project, "+
				"and firing on their schedule if they have one. If you removed or renamed them in your compose file, "+
				"you can run this command with the --remove-orphans flag to clean them up.", strings.Join(slices.Sorted(maps.Keys(orphans)), ", "))
		}
		return nil
	}
	// independent removals: one failing does not stop the others
	var eg errgroup.Group
	for _, op := range s.ensureJobsDown(ctx, &types.Project{Name: project.Name, Jobs: orphans}) {
		eg.Go(op)
	}
	return eg.Wait()
}
