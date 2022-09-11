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

import "time"

// EventStatus indicates the status of an action
type EventStatus int

const (
	// Working means that the current task is working
	Working EventStatus = iota
	// Done means that the current task is done
	Done
	// Error means that the current task has errored
	Error
	// Warning means that the current task has warning
	Warning
)

// Event represents a progress event.
type Event struct {
	ID         string
	ParentID   string
	Text       string
	Status     EventStatus
	StatusText string

	startTime time.Time
	endTime   time.Time
	spinner   *spinner
}

// ErrorMessageEvent creates a new Error Event with message
func ErrorMessageEvent(id string, msg string) Event {
	return NewEvent(id, Error, msg)
}

// ErrorEvent creates a new Error Event
func ErrorEvent(id string) Event {
	return NewEvent(id, Error, "Error")
}

// CreatingEvent creates a new Create in progress Event
func CreatingEvent(id string) Event {
	return NewEvent(id, Working, "Creating")
}

// StartingEvent creates a new Starting in progress Event
func StartingEvent(id string) Event {
	return NewEvent(id, Working, "Starting")
}

// StartedEvent creates a new Started in progress Event
func StartedEvent(id string) Event {
	return NewEvent(id, Done, "Started")
}

// Waiting creates a new waiting event
func Waiting(id string) Event {
	return NewEvent(id, Working, "Waiting")
}

// Healthy creates a new healthy event
func Healthy(id string) Event {
	return NewEvent(id, Done, "Healthy")
}

// Exited creates a new exited event
func Exited(id string) Event {
	return NewEvent(id, Done, "Exited")
}

// RestartingEvent creates a new Restarting in progress Event
func RestartingEvent(id string) Event {
	return NewEvent(id, Working, "Restarting")
}

// RestartedEvent creates a new Restarted in progress Event
func RestartedEvent(id string) Event {
	return NewEvent(id, Done, "Restarted")
}

// RunningEvent creates a new Running in progress Event
func RunningEvent(id string) Event {
	return NewEvent(id, Done, "Running")
}

// CreatedEvent creates a new Created (done) Event
func CreatedEvent(id string) Event {
	return NewEvent(id, Done, "Created")
}

// StoppingEvent creates a new Stopping in progress Event
func StoppingEvent(id string) Event {
	return NewEvent(id, Working, "Stopping")
}

// StoppedEvent creates a new Stopping in progress Event
func StoppedEvent(id string) Event {
	return NewEvent(id, Done, "Stopped")
}

// KillingEvent creates a new Killing in progress Event
func KillingEvent(id string) Event {
	return NewEvent(id, Working, "Killing")
}

// KilledEvent creates a new Killed in progress Event
func KilledEvent(id string) Event {
	return NewEvent(id, Done, "Killed")
}

// RemovingEvent creates a new Removing in progress Event
func RemovingEvent(id string) Event {
	return NewEvent(id, Working, "Removing")
}

// RemovedEvent creates a new removed (done) Event
func RemovedEvent(id string) Event {
	return NewEvent(id, Done, "Removed")
}

// NewEvent new event
func NewEvent(id string, status EventStatus, statusText string) Event {
	return Event{
		ID:         id,
		Status:     status,
		StatusText: statusText,
	}
}

func (e *Event) stop() {
	e.endTime = time.Now()
	e.spinner.Stop()
}
