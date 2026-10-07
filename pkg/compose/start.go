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
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Start(ctx context.Context, projectName string, options api.StartOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.start(ctx, strings.ToLower(projectName), options, nil)
	}, "start", s.events)
}

// start builds and executes a Start-only plan (epic #14081, lot 2): the
// semantic switchover from InDependencyOrder's imperative traversal
// (startService/waitDependencies) to the same reconciler/executor pair every
// other lifecycle command already uses. ScopeStart's planStartPhase works
// straight off the observed containers (see TestPlanStart_StartOnlyScope) --
// it never converges anything, matching start()'s own historical contract of
// starting what exists, never creating. listener is non-nil only when called
// from interactive up's own start phase (today's create()-then-start()
// sequence, unchanged by this PR -- see upDetached's doc comment): it streams
// pre_start/post_start hook logs into the attached session, exactly what
// newPlanExecutor's listener parameter exists for.
func (s *composeService) start(ctx context.Context, projectName string, options api.StartOptions, listener api.ContainerEventListener) error {
	project := options.Project
	if project == nil {
		containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
		if err != nil {
			return err
		}

		project, err = s.projectFromName(containers, projectName, options.AttachTo...)
		if err != nil {
			return err
		}
	}
	// resolve the model once: optional depends_on references left dangling by
	// profiles or service selection are pruned before the dependency graph
	// and the dependency waits read them
	project = project.WithoutUnresolvedOptionalDependencies()

	observed, err := s.collectObservedState(ctx, project)
	if err != nil {
		return err
	}

	plan, err := reconcile(ctx, project, observed, ReconcileOptions{Scope: ScopeStart}, s.prompt)
	if err != nil {
		return err
	}

	// Must run against the pre-execution snapshot: observed only labels a
	// container Running if it already was one before this plan touched
	// anything, same reasoning as upDetached.
	emitRunningEvents(project, observed, plan, s.events)

	// start()'s own dependency waits used to thread WaitTimeout through every
	// wait unconditionally (InDependencyOrder → startService, regardless of
	// options.Wait -- --wait-timeout alone, with no --wait, is a legal CLI
	// combination nothing rejects). exec.waitTimeout reproduces that for the
	// plan's own OpWaitCondition nodes, each with its own fresh window
	// starting when that wait begins -- see upDetached's identical reasoning.
	exec := s.newPlanExecutor(project, observed, listener)
	exec.waitTimeout = options.WaitTimeout

	if err := exec.run(ctx, plan); err != nil {
		// Same distinction as upDetached: ctx.Err() == nil rules out ctx's
		// own external deadline (unrelated to --wait-timeout) having fired
		// during exec.run, so a DeadlineExceeded here can only be one of the
		// plan's own OpWaitCondition nodes timing out.
		if options.WaitTimeout > 0 && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			if options.Wait {
				return fmt.Errorf("application not healthy after %s", options.WaitTimeout)
			}
			// No --wait: this is the dependency-timeout failure
			// waitDependencies already reports as "timeout waiting for
			// dependencies" elsewhere -- matching that established,
			// actionable message instead of leaking the raw
			// "context deadline exceeded" a user never configured in those
			// terms.
			return errors.New("timeout waiting for dependencies")
		}
		return err
	}

	if !options.Wait {
		return nil
	}

	// origCtx is kept so the two DeadlineExceeded checks below can tell this
	// fresh window expiring (origCtx still fine) apart from origCtx's own,
	// independent deadline propagating through the derived one (origCtx
	// already done) -- same reasoning as upDetached.
	origCtx := ctx
	if options.WaitTimeout > 0 {
		withTimeout, cancel := context.WithTimeout(ctx, options.WaitTimeout)
		ctx = withTimeout
		defer cancel()
	}

	// getContainers filters on ConfigHashLabel presence (getDefaultFilters),
	// which every service container carries and hook runners deliberately do
	// not: pre_start runners never leak into this verification at the source
	// (isNotHookContainer downstream stays as defense-in-depth). ScopeStart
	// never creates a container, so this re-listing exists only to open a
	// fresh WaitTimeout window, not to discover new IDs the pre-execution
	// observed snapshot wouldn't have (contrast upDetached, which can create
	// or recreate containers).
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, true)
	if err != nil {
		if options.WaitTimeout > 0 && origCtx.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("application not healthy after %s", options.WaitTimeout)
		}
		return err
	}

	depends := types.DependsOnConfig{}
	for _, svc := range project.Services {
		depends[svc.Name] = types.ServiceDependency{
			Condition: getDependencyCondition(svc, project),
			Required:  true,
		}
	}
	if err := s.waitDependencies(ctx, project, project.Name, depends, containers, 0); err != nil {
		if options.WaitTimeout > 0 && origCtx.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("application not healthy after %s", options.WaitTimeout)
		}
		return err
	}

	return nil
}

// getDependencyCondition checks if service is depended on by other services
// with service_completed_successfully condition, and applies that condition
// instead, or --wait will never finish waiting for one-shot containers
func getDependencyCondition(service types.ServiceConfig, project *types.Project) string {
	for _, services := range project.Services {
		for dependencyService, dependencyConfig := range services.DependsOn {
			if dependencyService == service.Name && dependencyConfig.Condition == types.ServiceConditionCompletedSuccessfully {
				return types.ServiceConditionCompletedSuccessfully
			}
		}
	}
	return ServiceConditionRunningOrHealthy
}
