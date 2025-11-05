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
	"fmt"

	"github.com/docker/compose/v2/pkg/api"
)

type progressFunc func(context.Context) error

func Run(ctx context.Context, pf progressFunc, operation string, bus api.EventProcessor) error {
	bus.Start(ctx, operation)
	err := pf(ctx)
	bus.Done(operation, err != nil)
	return err
}

// errorEvent creates a new Error Resource with message
func errorEvent(id string, msg string) api.Resource {
	return api.Resource{
		ID:      id,
		Status:  api.Error,
		Text:    api.StatusError,
		Details: msg,
	}
}

// errorEventf creates a new Error Resource with format message
func errorEventf(id string, msg string, args ...any) api.Resource {
	return errorEvent(id, fmt.Sprintf(msg, args...))
}

// creatingEvent creates a new Create in progress Resource
func creatingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusCreating)
}

// startingEvent creates a new Starting in progress Resource
func startingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusStarting)
}

// startedEvent creates a new Started in progress Resource
func startedEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusStarted)
}

// waiting creates a new waiting event
func waiting(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusWaiting)
}

// healthy creates a new healthy event
func healthy(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusHealthy)
}

// exited creates a new exited event
func exited(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusExited)
}

// restartingEvent creates a new Restarting in progress Resource
func restartingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusRestarting)
}

// runningEvent creates a new Running in progress Resource
func runningEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusRunning)
}

// createdEvent creates a new Created (done) Resource
func createdEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusCreated)
}

// stoppingEvent creates a new Stopping in progress Resource
func stoppingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusStopping)
}

// stoppedEvent creates a new Stopping in progress Resource
func stoppedEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusStopped)
}

// killingEvent creates a new Killing in progress Resource
func killingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusKilling)
}

// killedEvent creates a new Killed in progress Resource
func killedEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusKilled)
}

// removingEvent creates a new Removing in progress Resource
func removingEvent(id string) api.Resource {
	return newEvent(id, api.Working, api.StatusRemoving)
}

// removedEvent creates a new removed (done) Resource
func removedEvent(id string) api.Resource {
	return newEvent(id, api.Done, api.StatusRemoved)
}

// buildingEvent creates a new Building in progress Resource
func buildingEvent(id string) api.Resource {
	return newEvent("Image "+id, api.Working, api.StatusBuilding)
}

// builtEvent creates a new built (done) Resource
func builtEvent(id string) api.Resource {
	return newEvent("Image "+id, api.Done, api.StatusBuilt)
}

// pullingEvent creates a new pulling (in progress) Resource
func pullingEvent(id string) api.Resource {
	return newEvent("Image "+id, api.Working, api.StatusPulling)
}

// pulledEvent creates a new pulled (done) Resource
func pulledEvent(id string) api.Resource {
	return newEvent("Image "+id, api.Done, api.StatusPulled)
}

// skippedEvent creates a new Skipped Resource
func skippedEvent(id string, reason string) api.Resource {
	return api.Resource{
		ID:     id,
		Status: api.Warning,
		Text:   "Skipped: " + reason,
	}
}

// newEvent new event
func newEvent(id string, status api.EventStatus, text string, reason ...string) api.Resource {
	r := api.Resource{
		ID:     id,
		Status: status,
		Text:   text,
	}
	if len(reason) > 0 {
		r.Details = reason[0]
	}
	return r
}

type ignore struct{}

func (q *ignore) Start(_ context.Context, _ string) {
}

func (q *ignore) Done(_ string, _ bool) {
}

func (q *ignore) On(_ ...api.Resource) {
}
