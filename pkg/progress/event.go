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

package progress

import (
	"context"
	"fmt"
)

// EventStatus indicates the status of an action
type EventStatus int

const (
	// Working means that the current task is working
	Working EventStatus = iota
	// Done means that the current task is done
	Done
	// Warning means that the current task has warning
	Warning
	// Error means that the current task has errored
	Error
)

const (
	StatusError      = "Error"
	StatusCreating   = "Creating"
	StatusStarting   = "Starting"
	StatusStarted    = "Started"
	StatusWaiting    = "Waiting"
	StatusHealthy    = "Healthy"
	StatusExited     = "Exited"
	StatusRestarting = "Restarting"
	StatusRestarted  = "Restarted"
	StatusRunning    = "Running"
	StatusCreated    = "Created"
	StatusStopping   = "Stopping"
	StatusStopped    = "Stopped"
	StatusKilling    = "Killing"
	StatusKilled     = "Killed"
	StatusRemoving   = "Removing"
	StatusRemoved    = "Removed"
	StatusBuilding   = "Building"
	StatusBuilt      = "Built"
	StatusPulling    = "Pulling"
	StatusPulled     = "Pulled"
	StatusCommitting = "Committing"
	StatusCommitted  = "Committed"
	StatusCopying    = "Copying"
	StatusCopied     = "Copied"
	StatusExporting  = "Exporting"
	StatusExported   = "Exported"
)

// Event represents a progress event.
type Event struct {
	ID       string
	ParentID string
	Text     string
	Details  string
	Status   EventStatus
	Current  int64
	Percent  int
	Total    int64
}

func (e *Event) StatusText() string {
	switch e.Status {
	case Working:
		return "Working"
	case Warning:
		return "Warning"
	case Done:
		return "Done"
	default:
		return "Error"
	}
}

// ErrorEvent creates a new Error Event with message
func ErrorEvent(id string, msg string) Event {
	return Event{
		ID:      id,
		Status:  Error,
		Text:    StatusError,
		Details: msg,
	}
}

// ErrorEventf creates a new Error Event with format message
func ErrorEventf(id string, msg string, args ...any) Event {
	return ErrorEvent(id, fmt.Sprintf(msg, args...))
}

// CreatingEvent creates a new Create in progress Event
func CreatingEvent(id string) Event {
	return NewEvent(id, Working, StatusCreating)
}

// StartingEvent creates a new Starting in progress Event
func StartingEvent(id string) Event {
	return NewEvent(id, Working, StatusStarting)
}

// StartedEvent creates a new Started in progress Event
func StartedEvent(id string) Event {
	return NewEvent(id, Done, StatusStarted)
}

// Waiting creates a new waiting event
func Waiting(id string) Event {
	return NewEvent(id, Working, StatusWaiting)
}

// Healthy creates a new healthy event
func Healthy(id string) Event {
	return NewEvent(id, Done, StatusHealthy)
}

// Exited creates a new exited event
func Exited(id string) Event {
	return NewEvent(id, Done, StatusExited)
}

// RestartingEvent creates a new Restarting in progress Event
func RestartingEvent(id string) Event {
	return NewEvent(id, Working, StatusRestarting)
}

// RestartedEvent creates a new Restarted in progress Event
func RestartedEvent(id string) Event {
	return NewEvent(id, Done, StatusRestarted)
}

// RunningEvent creates a new Running in progress Event
func RunningEvent(id string) Event {
	return NewEvent(id, Done, StatusRunning)
}

// CreatedEvent creates a new Created (done) Event
func CreatedEvent(id string) Event {
	return NewEvent(id, Done, StatusCreated)
}

// StoppingEvent creates a new Stopping in progress Event
func StoppingEvent(id string) Event {
	return NewEvent(id, Working, StatusStopping)
}

// StoppedEvent creates a new Stopping in progress Event
func StoppedEvent(id string) Event {
	return NewEvent(id, Done, StatusStopped)
}

// KillingEvent creates a new Killing in progress Event
func KillingEvent(id string) Event {
	return NewEvent(id, Working, StatusKilling)
}

// KilledEvent creates a new Killed in progress Event
func KilledEvent(id string) Event {
	return NewEvent(id, Done, StatusKilled)
}

// RemovingEvent creates a new Removing in progress Event
func RemovingEvent(id string) Event {
	return NewEvent(id, Working, StatusRemoving)
}

// RemovedEvent creates a new removed (done) Event
func RemovedEvent(id string) Event {
	return NewEvent(id, Done, StatusRemoved)
}

// BuildingEvent creates a new Building in progress Event
func BuildingEvent(id string) Event {
	return NewEvent("Image "+id, Working, StatusBuilding)
}

// BuiltEvent creates a new built (done) Event
func BuiltEvent(id string) Event {
	return NewEvent("Image "+id, Done, StatusBuilt)
}

// PullingEvent creates a new pulling (in progress) Event
func PullingEvent(id string) Event {
	return NewEvent("Image "+id, Working, StatusPulling)
}

// PulledEvent creates a new pulled (done) Event
func PulledEvent(id string) Event {
	return NewEvent("Image "+id, Done, StatusPulled)
}

// SkippedEvent creates a new Skipped Event
func SkippedEvent(id string, reason string) Event {
	return Event{
		ID:     id,
		Status: Warning,
		Text:   "Skipped: " + reason,
	}
}

// NewEvent new event
func NewEvent(id string, status EventStatus, text string) Event {
	return Event{
		ID:     id,
		Status: status,
		Text:   text,
	}
}

// EventProcessor is notified about Compose operations and tasks
type EventProcessor interface {
	// Start is triggered as a Compose operation is starting with context
	Start(ctx context.Context, operation string)
	// On notify about (sub)task and progress processing operation
	On(events ...Event)
	// Done is triggered as a Compose operation completed
	Done(operation string, success bool)
}
