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
	"maps"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	extensionclient "github.com/moby/extensions/client"
	containerType "github.com/moby/moby/api/types/container"
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

// ensureJobsDown removes the project's jobs from the engine. Run history is
// kept by default (RunsRemoval left empty); each removal tolerates the job
// already being gone, matching removeResource's NotFound handling.
func (s *composeService) ensureJobsDown(ctx context.Context, project *types.Project) []downOp {
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
