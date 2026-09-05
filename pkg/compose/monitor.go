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

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

type monitor struct {
	apiClient client.APIClient
	project   string
	// services tells us which service to consider and those we can ignore, maybe ran by a concurrent compose command
	services  map[string]bool
	listeners []api.ContainerEventListener
}

func newMonitor(apiClient client.APIClient, project string) *monitor {
	return &monitor{
		apiClient: apiClient,
		project:   project,
		services:  map[string]bool{},
	}
}

func (c *monitor) withServices(services []string) {
	for _, name := range services {
		c.services[name] = true
	}
}

// Start runs monitor to detect application events and return after termination
//
// The monitor consumes the engine's event stream, filtered on this project's
// containers, and has to interpret event sequences that depend on how a
// container terminates or comes back. The sequences below were mapped from
// moby's daemon sources and verified against a live engine (v29.x);
// meaningful event attributes are in parentheses:
//
//	container process exits on its own, whatever the exit code:
//	    die (exitCode) — and nothing else: no stop event is emitted
//
//	restart policy in action, the daemon restarts the container after exit:
//	    die (exitCode) → start → die → start → ...
//	    No restart event is ever emitted for policy-driven restarts, and no
//	    event at all while the container waits in restart backoff (status
//	    "restarting", no process running); ContainerInspect reports
//	    State.Restarting=true in that window.
//
//	docker stop (or compose stop) on a running container:
//	    kill (signal=stop signal, SIGTERM by default)
//	    [→ kill (signal=9, i.e. SIGKILL) when the grace period expires]
//	    → stop + die (exitCode)
//	    The relative order of stop and die is not guaranteed: stop is emitted
//	    by the ContainerStop call completing, die by the asynchronous exit
//	    notification from containerd. A manual stop cancels the restart policy.
//
//	docker stop on a container in restart backoff (see #13985):
//	    stop — and nothing else: with no process to signal, the daemon skips
//	    the kill event, no die is ever produced, and the pending restart is
//	    cancelled.
//
//	docker restart, and the ContainerRestart API in general (also used by
//	compose watch sync+restart, see #13161):
//	    kill → stop + die → start → restart
//	    A transient stop/die pair is emitted even though the container is
//	    coming back. State.Restarting is never set on this path — the only
//	    daemon code setting it is the restart-policy one — so the container is
//	    briefly reported as "exited" between die and start, then "running"
//	    again (https://github.com/moby/moby/issues/45538); an inspection
//	    triggered by the die event usually, but not reliably, lands after the
//	    new start. The restart action fires last, after start.
//
//	docker kill:
//	    kill (signal=9, i.e. SIGKILL) → die (exitCode) — no stop event
//	    A non-fatal signal (docker kill -s HUP) emits kill alone, with no die
//	    unless the process exits.
//
//	OOM-killed container:
//	    oom → die (exitCode, typically 137)
//	    The die event carries no OOM-specific attribute, and a restart policy
//	    applies as for any other exit (start follows). An oom can also occur
//	    with no die at all, when the kernel kills a child process and the
//	    container's main process survives.
//
//	docker rm -f on a running container:
//	    kill → die → destroy
//
//	docker rm on a stopped container, docker rm -f on a restarting one:
//	    destroy — and nothing else
//
// Other actions (pause, unpause, attach, exec_*, health_status: *, ...) may
// show up in the stream but don't affect container tracking. Note that
// exec_create, exec_start and health_status actions embed variable data in
// the action name itself (e.g. "health_status: healthy"), which would break
// exact matching. On daemon shutdown containers are stopped (kill/die/stop)
// and the event stream usually delivers those events before it breaks, so a
// daemon restart is typically observed as a per-container kill/die/stop
// sequence followed by Events returning an error. A container that dies
// while the daemon itself is down never gets a die event at all: on daemon
// restore it is just recorded as exited, and containers the restore restarts
// per policy only emit start.
func (c *monitor) Start(ctx context.Context) error {
	// containers is the set of container IDs the application is based on
	containers, err := c.initialContainers(ctx)
	if err != nil {
		return err
	}
	// restarting tracks containers which exited but are configured to restart on exit
	restarting := utils.Set[string]{}

	res := c.apiClient.Events(ctx, client.EventsListOptions{
		Filters: projectFilter(c.project).Add("type", "container").Add("label", oneOffFilter(false)),
	})
	for {
		if len(containers) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-res.Err:
			return err
		case event := <-res.Messages:
			if !c.watched(event.Actor.Attributes[api.ServiceLabel]) {
				continue
			}
			ctr, err := c.getContainerSummary(event)
			if err != nil {
				return err
			}

			switch event.Action {
			case events.ActionCreate:
				c.onContainerCreate(event, ctr, containers)
			case events.ActionStart:
				c.onContainerStart(event, ctr, containers, restarting)
			case events.ActionRestart:
				c.onContainerRestart(event, ctr)
			case events.ActionDie:
				err := c.onContainerDie(ctx, event, ctr, containers, restarting)
				if err != nil {
					return err
				}
			}
		}
	}
}

// initialContainers collects the application's containers at startup,
// restricted to the services this monitor watches
func (c *monitor) initialContainers(ctx context.Context) (utils.Set[string], error) {
	initialState, err := c.apiClient.ContainerList(ctx, client.ContainerListOptions{
		All: true,
		Filters: projectFilter(c.project).Add("label",
			oneOffFilter(false),
			api.ConfigHashLabel,
		),
	})
	if err != nil {
		return nil, err
	}
	containers := utils.Set[string]{}
	for _, ctr := range initialState.Items {
		if c.watched(ctr.Labels[api.ServiceLabel]) {
			containers.Add(ctr.ID)
		}
	}
	return containers, nil
}

// watched tells whether a service's containers are watched by this monitor.
// An empty service set means "the whole application".
func (c *monitor) watched(service string) bool {
	return len(c.services) == 0 || c.services[service]
}

// notify broadcasts a container event to the registered listeners
func (c *monitor) notify(event api.ContainerEvent) {
	for _, listener := range c.listeners {
		listener(event)
	}
}

func (c *monitor) onContainerCreate(event events.Message, ctr *api.ContainerSummary, containers utils.Set[string]) {
	if c.watched(ctr.Labels[api.ServiceLabel]) {
		containers.Add(ctr.ID)
	}
	evtType := api.ContainerEventCreated
	if _, ok := ctr.Labels[api.ContainerReplaceLabel]; ok {
		evtType = api.ContainerEventRecreated
	}
	c.notify(newContainerEvent(event.TimeNano, ctr, evtType))
	logrus.Debugf("container %s created", ctr.Name)
}

func (c *monitor) onContainerStart(event events.Message, ctr *api.ContainerSummary, containers, restarting utils.Set[string]) {
	if restarting.Has(ctr.ID) {
		logrus.Debugf("container %s restarted", ctr.Name)
		c.notify(newContainerEvent(event.TimeNano, ctr, api.ContainerEventStarted, func(e *api.ContainerEvent) {
			e.Restarting = true
		}))
	} else {
		logrus.Debugf("container %s started", ctr.Name)
		c.notify(newContainerEvent(event.TimeNano, ctr, api.ContainerEventStarted))
	}
	if c.watched(ctr.Labels[api.ServiceLabel]) {
		containers.Add(ctr.ID)
	}
}

func (c *monitor) onContainerRestart(event events.Message, ctr *api.ContainerSummary) {
	c.notify(newContainerEvent(event.TimeNano, ctr, api.ContainerEventRestarted))
	logrus.Debugf("container %s restarted", ctr.Name)
}

func (c *monitor) onContainerDie(ctx context.Context, event events.Message, ctr *api.ContainerSummary, containers, restarting utils.Set[string]) error {
	logrus.Debugf("container %s exited with code %d", ctr.Name, ctr.ExitCode)
	inspect, err := c.apiClient.ContainerInspect(ctx, event.Actor.ID, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		// Source is already removed
	} else if err != nil {
		return err
	}

	if inspect.Container.State != nil && (inspect.Container.State.Restarting || inspect.Container.State.Running) {
		// State.Restarting is set by engine when container is configured to restart on exit
		// on ContainerRestart it doesn't (see https://github.com/moby/moby/issues/45538)
		// container state still is reported as "running"
		logrus.Debugf("container %s is restarting", ctr.Name)
		restarting.Add(ctr.ID)
		c.notify(newContainerEvent(event.TimeNano, ctr, api.ContainerEventExited, func(e *api.ContainerEvent) {
			e.Restarting = true
		}))
		return nil
	}

	c.notify(newContainerEvent(event.TimeNano, ctr, api.ContainerEventExited))
	containers.Remove(ctr.ID)
	return nil
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
		Project: c.project,
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
