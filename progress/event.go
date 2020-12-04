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
func ErrorMessageEvent(ID string, msg string) Event {
	return NewEvent(ID, Error, msg)
}

// ErrorEvent creates a new Error Event
func ErrorEvent(ID string) Event {
	return NewEvent(ID, Error, "Error")
}

// CreatingEvent creates a new Create in progress Event
func CreatingEvent(ID string) Event {
	return NewEvent(ID, Working, "Creating")
}

// StartingEvent creates a new Starting in progress Event
func StartingEvent(ID string) Event {
	return NewEvent(ID, Working, "Starting")
}

// StartedEvent creates a new Started in progress Event
func StartedEvent(ID string) Event {
	return NewEvent(ID, Done, "Started")
}

// RunningEvent creates a new Running in progress Event
func RunningEvent(ID string) Event {
	return NewEvent(ID, Done, "Running")
}

// CreatedEvent creates a new Created (done) Event
func CreatedEvent(ID string) Event {
	return NewEvent(ID, Done, "Created")
}

// StoppingEvent stops a new Removing in progress Event
func StoppingEvent(ID string) Event {
	return NewEvent(ID, Working, "Stopping")
}

// RemovingEvent creates a new Removing in progress Event
func RemovingEvent(ID string) Event {
	return NewEvent(ID, Working, "Removing")
}

// RemovedEvent creates a new removed (done) Event
func RemovedEvent(ID string) Event {
	return NewEvent(ID, Done, "Removed")
}

// NewEvent new event
func NewEvent(ID string, status EventStatus, statusText string) Event {
	return Event{
		ID:         ID,
		Status:     status,
		StatusText: statusText,
	}
}

func (e *Event) stop() {
	e.endTime = time.Now()
	e.spinner.Stop()
}
