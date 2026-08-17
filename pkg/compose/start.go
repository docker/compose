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
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Start(ctx context.Context, projectName string, options api.StartOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.start(ctx, strings.ToLower(projectName), options, nil)
	}, "start", s.events)
}

func (s *composeService) start(ctx context.Context, projectName string, options api.StartOptions, listener api.ContainerEventListener) error {
	project := options.Project
	if project == nil {
		var containers Containers
		containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
		if err != nil {
			return err
		}

		project, err = s.projectFromName(containers, projectName, options.AttachTo...)
		if err != nil {
			return err
		}
	}

	if listener == nil {
		// Start-only plan profile (see the start-in-plan convergence epic):
		// existing containers are started as they are, no resource touched.
		// The listener-driven path below remains for the interactive up,
		// until the session moves to executor hooks (phase 2).
		return s.startWithPlan(ctx, project, options)
	}

	res, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		Filters: projectFilter(project.Name).Add("label", oneOffFilter(false)),
		All:     true,
	})
	if err != nil {
		return err
	}
	containers := Containers(res.Items)

	err = InDependencyOrder(ctx, project, func(c context.Context, name string) error {
		service, err := project.GetService(name)
		if err != nil {
			return err
		}

		return s.startService(ctx, project, service, containers, listener, options.WaitTimeout)
	})
	if err != nil {
		return err
	}

	if options.Wait {
		return s.waitStarted(ctx, project, options.WaitTimeout)
	}

	return nil
}

// startWithPlan runs `compose start` through the plan engine: a start-only
// plan (dependency-condition waits, pre_start hooks, container starts) over
// the containers as they exist — never creating, recreating or removing
// anything.
func (s *composeService) startWithPlan(ctx context.Context, project *types.Project, options api.StartOptions) error {
	observed, err := s.collectObservedState(ctx, project)
	if err != nil {
		return err
	}

	empty := true
	for _, ocs := range observed.Containers {
		if len(ocs) > 0 {
			empty = false
			break
		}
	}
	if empty {
		for _, name := range sortedKeys(project.Services) {
			svc := project.Services[name]
			if svc.GetScale() > 0 {
				return fmt.Errorf("service %q has no container to start", name)
			}
		}
		return nil
	}

	plan, err := reconcile(ctx, project, observed, ReconcileOptions{
		Start: &startPhaseOptions{Only: true, WaitTimeout: options.WaitTimeout},
	}, s.prompt)
	if err != nil {
		return err
	}
	if err := s.executePlan(ctx, project, observed, plan); err != nil {
		return err
	}

	if options.Wait {
		return s.waitStarted(ctx, project, options.WaitTimeout)
	}
	return nil
}

// waitStarted blocks until every service of the project is running (or
// healthy, or completed for one-shot services), implementing the --wait
// barrier of `up` and `start`.
func (s *composeService) waitStarted(ctx context.Context, project *types.Project, timeout time.Duration) error {
	res, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		Filters: projectFilter(project.Name).Add("label", oneOffFilter(false)),
		All:     true,
	})
	if err != nil {
		return err
	}
	containers := Containers(res.Items)

	depends := types.DependsOnConfig{}
	for _, svc := range project.Services {
		depends[svc.Name] = types.ServiceDependency{
			Condition: getDependencyCondition(svc, project),
			Required:  true,
		}
	}
	if timeout > 0 {
		withTimeout, cancel := context.WithTimeout(ctx, timeout)
		ctx = withTimeout
		defer cancel()
	}

	err = s.waitDependencies(ctx, project, project.Name, depends, containers, 0)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("application not healthy after %s", timeout)
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
