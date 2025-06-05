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
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

type monitor struct {
	api     client.APIClient
	project *types.Project
	// services tells us which service to consider and those we can ignore, maybe ran by a concurrent compose command
	services  map[string]bool
	listeners []api.ContainerEventListener
}

func newMonitor(api client.APIClient, project *types.Project) *monitor {
	services := map[string]bool{}
	if project != nil {
		for name := range project.Services {
			services[name] = true
		}
	}
	return &monitor{
		api:      api,
		project:  project,
		services: services,
	}
}

// Start runs monitor to detect application events and return after termination
//
//nolint:gocyclo
func (c *monitor) Start(ctx context.Context) error {
	// collect initial application container
	initialState, err := c.api.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			projectFilter(c.project.Name),
			oneOffFilter(false),
			hasConfigHashLabel(),
		),
	})
	if err != nil {
		return err
	}

	// containers is the set if container IDs the application is based on
	containers := utils.Set[string]{}
	for _, ctr := range initialState {
		if len(c.services) == 0 || c.services[ctr.Labels[api.ServiceLabel]] {
			containers.Add(ctr.ID)
		}
	}

	restarting := utils.Set[string]{}

	evtCh, errCh := c.api.Events(ctx, events.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			projectFilter(c.project.Name)),
	})
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case event := <-evtCh:
			if !c.services[event.Actor.Attributes[api.ServiceLabel]] {
				continue
			}
			ctr, err := c.getContainerSummary(event)
			if err != nil {
				return err
			}

			switch event.Action {
			case events.ActionCreate:
				containers.Add(ctr.ID)
				for _, listener := range c.listeners {
					listener(newContainerEvent(event.TimeNano, ctr, api.ContainerEventCreated))
				}
				logrus.Debugf("container %s created", ctr.Name)
			case events.ActionStart:
				restarted := restarting.Has(ctr.ID)
				for _, listener := range c.listeners {
					listener(newContainerEvent(event.TimeNano, ctr, api.ContainerEventStarted, func(e *api.ContainerEvent) {
						e.Restarting = restarted
					}))
				}
				if restarted {
					logrus.Debugf("container %s restarted", ctr.Name)
				} else {
					logrus.Debugf("container %s started", ctr.Name)
				}
				containers.Add(ctr.ID)
			case events.ActionRestart:
				for _, listener := range c.listeners {
					listener(newContainerEvent(event.TimeNano, ctr, api.ContainerEventRestarted))
				}
				logrus.Debugf("container %s restarted", ctr.Name)
			case events.ActionStop:
				// when a container is in restarting phase, and we stop the application (abort-on-container-exit)
				// we won't get any additional start+die events, just this stop as a proof container is down
				logrus.Debugf("container %s stopped", ctr.Name)
				containers.Remove(ctr.ID)
			case events.ActionDie:
				logrus.Debugf("container %s exited with code %d", ctr.Name, ctr.ExitCode)
				inspect, err := c.api.ContainerInspect(ctx, event.Actor.ID)
				if errdefs.IsNotFound(err) {
					// Source is already removed
				} else if err != nil {
					return err
				}

				if inspect.State != nil && inspect.State.Restarting || inspect.State.Running {
					// State.Restarting is set by engine when container is configured to restart on exit
					// on ContainerRestart it doesn't (see https://github.com/moby/moby/issues/45538)
					// container state still is reported as "running"
					logrus.Debugf("container %s is restarting", ctr.Name)
					restarting.Add(ctr.ID)
					for _, listener := range c.listeners {
						listener(newContainerEvent(event.TimeNano, ctr, api.ContainerEventExited, func(e *api.ContainerEvent) {
							e.Restarting = true
						}))
					}
				} else {
					for _, listener := range c.listeners {
						listener(newContainerEvent(event.TimeNano, ctr, api.ContainerEventExited))
					}
					containers.Remove(ctr.ID)
				}
			}
		}
		if len(containers) == 0 {
			return nil
		}
	}
}

func newContainerEvent(timeNano int64, ctr *api.ContainerSummary, eventType int, opts ...func(e *api.ContainerEvent)) api.ContainerEvent {
	name := ctr.Name
	defaultName := getDefaultContainerName(ctr.Project, ctr.Labels[api.ServiceLabel], ctr.Labels[api.ContainerNumberLabel])
	if name == defaultName {
		// remove project- prefix
		name = name[len(ctr.Project)+1:]
	}

	event := api.ContainerEvent{
		Type:      eventType,
		Container: ctr,
		Time:      timeNano,
		Source:    name,
		ID:        ctr.ID,
		Service:   ctr.Service,
		ExitCode:  ctr.ExitCode,
	}
	for _, opt := range opts {
		opt(&event)
	}
	return event
}

func (c *monitor) getContainerSummary(event events.Message) (*api.ContainerSummary, error) {
	ctr := &api.ContainerSummary{
		ID:      event.Actor.ID,
		Name:    event.Actor.Attributes["name"],
		Project: c.project.Name,
		Service: event.Actor.Attributes[api.ServiceLabel],
		Labels:  event.Actor.Attributes, // More than just labels, but that'c the closest the API gives us
	}
	if ec, ok := event.Actor.Attributes["exitCode"]; ok {
		exitCode, err := strconv.Atoi(ec)
		if err != nil {
			return nil, err
		}
		ctr.ExitCode = exitCode
	}
	return ctr, nil
}

func (c *monitor) withListener(listener api.ContainerEventListener) {
	c.listeners = append(c.listeners, listener)
}
