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
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// This file gathers the waits on depends_on conditions: the polling of a
// dependency's containers (waitDependency and the per-condition checks), shared
// by the plan's OpWaitCondition nodes and by the commands that wait outside a
// plan (waitDependencies: restart, run and the --wait verification of up and
// start), and the gate deciding which dependencies need a wait at all.

// ServiceConditionRunningOrHealthy is a service condition on status running or healthy
const ServiceConditionRunningOrHealthy = "running_or_healthy"

func (s *composeService) waitDependencies(ctx context.Context, project *types.Project, dependant string, dependencies types.DependsOnConfig, containers Containers) error {
	origCtx := ctx
	eg, ctx := errgroup.WithContext(ctx)
	for dep, config := range dependencies {
		if shouldWait, err := shouldWaitForDependency(dep, config, project); err != nil {
			return err
		} else if !shouldWait {
			continue
		}

		waitingFor := containers.filter(isService(dep), isNotOneOff, isNotHookContainer)
		s.events.On(containerEvents(waitingFor, progress.Waiting)...)
		if len(waitingFor) == 0 {
			if config.Required {
				return fmt.Errorf("%s is missing dependency %s", dependant, dep)
			}
			logrus.Warnf("%s is missing dependency %s", dependant, dep)
			continue
		}

		eg.Go(func() error {
			return s.waitDependency(ctx, origCtx, dependant, dep, config, waitingFor)
		})
	}
	err := eg.Wait()
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout waiting for dependencies")
	}
	return err
}

// waitDependency polls the dependency's containers until its depends_on
// condition is satisfied (done), definitively failed (err), or ctx is
// cancelled. Each check reports (done, err): (false, nil) means keep polling.
// origCtx is ctx as it stood before the caller wrapped it with this wait's
// own timeout (if any: waitDependencies sets none, the plan's wait nodes do,
// see execBoundedWaitCondition) -- comparing origCtx.Err() against ctx.Err() at the
// moment ctx.Done() fires is how this tells apart a DeadlineExceeded this
// specific call's own wrap is responsible for from one inherited from an
// ancestor context the caller never touched. A bare "did the caller
// configure a timeout at all" flag isn't enough: if origCtx itself already
// carries an earlier deadline, wrapping it with context.WithTimeout still
// makes both origCtx and ctx report DeadlineExceeded once that earlier
// deadline fires, even though this call's own timeout never actually
// elapsed -- only origCtx.Err() == nil proves this wrap's own timeout is
// the one that fired. Only that case is safe to tolerate on an optional
// dependency; an inherited deadline means something else entirely (an
// external deadline, a test harness timeout) fired, and must keep
// propagating regardless of Required.
func (s *composeService) waitDependency(ctx, origCtx context.Context, dependant, dep string, config types.ServiceDependency, waitingFor Containers) error {
	_, err := s.waitDependencyReached(ctx, origCtx, dependant, dep, config, waitingFor)
	return err
}

// waitDependencyReached is waitDependency, which also tells whether the wait is
// over because the dependency is ready (or an optional one was given up on),
// as opposed to a plain cancellation of ctx, which ends it silently.
func (s *composeService) waitDependencyReached(ctx, origCtx context.Context, dependant, dep string, config types.ServiceDependency, waitingFor Containers) (bool, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	// running_or_healthy accepts a running container without a healthcheck:
	// checked right after it starts, one that exits at once would pass.
	checkOnEntry := config.Condition == types.ServiceConditionHealthy || config.Condition == types.ServiceConditionCompletedSuccessfully
	for first := true; ; first = false {
		// a tick and the end of ctx can be ready together, and select would
		// then pick either: ctx wins, so that a canceled wait is never run
		// through one more check
		if !first || !checkOnEntry {
			select {
			case <-ticker.C:
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			// An expired deadline is precisely the failure this wait is meant
			// to detect for a required dependency; only a plain cancellation
			// (Ctrl-C) stays silent either way. An optional dependency
			// tolerates a timeout exactly like it tolerates any other
			// definitive failure the check* functions below report —
			// skipped, not aborted — but only this call's OWN timeout: an
			// inherited deadline is a different, real failure the caller
			// must still see.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				if !config.Required && origCtx.Err() == nil {
					s.events.On(containerReasonEvents(waitingFor, progress.Skipped,
						fmt.Sprintf("optional dependency %q did not satisfy %q before the timeout", dep, config.Condition))...)
					logrus.Warnf("optional dependency %q did not satisfy %q before the timeout", dep, config.Condition)
					return true, nil
				}
				return false, ctx.Err()
			}
			return false, nil
		}
		var (
			done bool
			err  error
		)
		switch config.Condition {
		case ServiceConditionRunningOrHealthy:
			done, err = s.checkDependencyRunningOrHealthy(ctx, dep, config, waitingFor)
		case types.ServiceConditionHealthy:
			done, err = s.checkDependencyHealthy(ctx, dep, config, waitingFor)
		case types.ServiceConditionCompletedSuccessfully:
			done, err = s.checkDependencyCompleted(ctx, dep, config, waitingFor)
		default:
			// Every condition this switch doesn't handle explicitly ends up
			// here: ServiceConditionStarted is filtered out before this
			// function is ever called (shouldWaitForDependency — "already
			// ensured by the dependency order"), so nothing reaching this
			// branch is ever a value this runtime actually understands.
			// That covers a compose file with an unrecognized condition
			// (rejected by schema.Validate before it can even get this far
			// — see unsupported_attributes.go's comment on
			// valueConditionalAttributes) as well as the two situations
			// that never go through schema validation at all: a project
			// rebuilt from live container labels (see projectFromName), or
			// one written by an older/newer Compose version using a
			// condition value this build doesn't recognize.
			logrus.Warnf("service %q: unsupported depends_on condition %q, skipping wait for %q", dependant, config.Condition, dep)
			return true, nil
		}
		if done || err != nil {
			return done, err
		}
	}
}

// checkDependencyRunningOrHealthy and checkDependencyHealthy skip an optional
// dependency whose inspection fails. A failure caused by ctx being done says
// nothing about the dependency though: reporting it as "skipped" would turn a
// cancellation or an expired deadline into a successful wait. They report
// "not done yet" instead, so that waitDependency's next iteration sees
// ctx.Done() and decides what the end of the wait means. A genuine failure that
// coincides with the end of the wait goes unreported on purpose: the wait is
// over anyway.
func (s *composeService) checkDependencyRunningOrHealthy(ctx context.Context, dep string, config types.ServiceDependency, waitingFor Containers) (bool, error) {
	isHealthy, err := s.isServiceHealthy(ctx, waitingFor, true)
	if err != nil {
		if !config.Required {
			if ctx.Err() != nil {
				return false, nil
			}
			s.events.On(containerReasonEvents(waitingFor, progress.Skipped,
				fmt.Sprintf("optional dependency %q is not running or is unhealthy", dep))...)
			logrus.Warnf("optional dependency %q is not running or is unhealthy: %s", dep, err.Error())
			return true, nil
		}
		return false, err
	}
	if isHealthy {
		s.events.On(containerEvents(waitingFor, progress.Healthy)...)
	}
	return isHealthy, nil
}

func (s *composeService) checkDependencyHealthy(ctx context.Context, dep string, config types.ServiceDependency, waitingFor Containers) (bool, error) {
	isHealthy, err := s.isServiceHealthy(ctx, waitingFor, false)
	if err != nil {
		if !config.Required {
			if ctx.Err() != nil {
				return false, nil
			}
			s.events.On(containerReasonEvents(waitingFor, progress.Skipped,
				fmt.Sprintf("optional dependency %q failed to start", dep))...)
			logrus.Warnf("optional dependency %q failed to start: %s", dep, err.Error())
			return true, nil
		}
		if !errors.Is(err, context.Canceled) {
			// a failure caused by a canceled ctx says nothing about the
			// dependency: its rows are not marked failed for it. The error is
			// tested, not the ctx: a genuine failure (unhealthy, exited)
			// found just as another node cancels the phase is still reported
			s.events.On(containerEvents(waitingFor, func(s string) api.Resource {
				return errorEventf(s, "dependency %s failed to start", dep)
			})...)
		}
		return false, fmt.Errorf("dependency failed to start: %w", err)
	}
	if isHealthy {
		s.events.On(containerEvents(waitingFor, progress.Healthy)...)
	}
	return isHealthy, nil
}

func (s *composeService) checkDependencyCompleted(ctx context.Context, dep string, config types.ServiceDependency, waitingFor Containers) (bool, error) {
	isExited, code, err := s.isServiceCompleted(ctx, waitingFor)
	if err != nil {
		return false, err
	}
	if !isExited {
		return false, nil
	}
	if code == 0 {
		s.events.On(containerEvents(waitingFor, progress.Exited)...)
		return true, nil
	}

	messageSuffix := fmt.Sprintf("%q didn't complete successfully: exit %d", dep, code)
	if !config.Required {
		// optional -> mark as skipped & don't propagate error
		s.events.On(containerReasonEvents(waitingFor, progress.Skipped,
			"optional dependency "+messageSuffix)...)
		logrus.Warnf("optional dependency %s", messageSuffix)
		return true, nil
	}

	msg := "service " + messageSuffix
	s.events.On(containerEvents(waitingFor, func(s string) api.Resource {
		return errorEventf(s, "service %s", messageSuffix)
	})...)
	return false, errors.New(msg)
}

func shouldWaitForDependency(serviceName string, dependencyConfig types.ServiceDependency, project *types.Project) (bool, error) {
	if dependencyConfig.Condition == types.ServiceConditionStarted {
		// already ensured by the dependency order
		return false, nil
	}
	if service, err := project.GetService(serviceName); err != nil {
		for _, ds := range project.DisabledServices {
			if ds.Name == serviceName {
				// don't wait for disabled service (--no-deps)
				return false, nil
			}
		}
		return false, err
	} else if service.GetScale() == 0 {
		// don't wait for the dependency which configured to have 0 containers running
		return false, nil
	} else if service.Provider != nil {
		// don't wait for provider services
		return false, nil
	}
	return true, nil
}

func (s *composeService) isServiceHealthy(ctx context.Context, containers Containers, fallbackRunning bool) (bool, error) {
	for _, ctr := range containers {
		res, err := s.apiClient().ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
		if err != nil {
			return false, err
		}
		ctr := res.Container
		name := ctr.Name[1:]

		if ctr.State.Status == container.StateExited {
			return false, fmt.Errorf("container %s exited (%d)", name, ctr.State.ExitCode)
		}

		noHealthcheck := ctr.Config.Healthcheck == nil || (len(ctr.Config.Healthcheck.Test) > 0 && ctr.Config.Healthcheck.Test[0] == "NONE")
		if noHealthcheck && fallbackRunning {
			// Container does not define a health check, but we can fall back to "running" state
			return ctr.State != nil && ctr.State.Status == container.StateRunning, nil
		}

		if ctr.State == nil || ctr.State.Health == nil {
			return false, fmt.Errorf("container %s has no healthcheck configured", name)
		}
		switch ctr.State.Health.Status {
		case container.Healthy:
			// Continue by checking the next container.
		case container.Unhealthy:
			return false, fmt.Errorf("container %s is unhealthy", name)
		case container.Starting:
			return false, nil
		default:
			return false, fmt.Errorf("container %s had unexpected health status %q", name, ctr.State.Health.Status)
		}
	}
	return true, nil
}

func (s *composeService) isServiceCompleted(ctx context.Context, containers Containers) (bool, int, error) {
	if s.dryRun {
		// dry-run never actually starts the dependency's container, so it
		// can never observe a real "exited" state: simulate immediate
		// success instead of polling forever.
		return true, 0, nil
	}
	for _, ctr := range containers {
		res, err := s.apiClient().ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
		if err != nil {
			return false, 0, err
		}
		if res.Container.State != nil && res.Container.State.Status == container.StateExited {
			return true, res.Container.State.ExitCode, nil
		}
	}
	return false, 0, nil
}
