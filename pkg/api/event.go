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

package api

import (
	"context"
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

// ResourceCompose is a special resource ID used when event applies to all resources in the application
const ResourceCompose = "Compose"

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

// Resource represents status change and progress for a compose resource.
type Resource struct {
	ID       string
	ParentID string
	Text     string
	Details  string
	Status   EventStatus
	Current  int64
	Percent  int
	Total    int64
}

func (e *Resource) StatusText() string {
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

// EventProcessor is notified about Compose operations and tasks
type EventProcessor interface {
	// Start is triggered as a Compose operation is starting with context
	Start(ctx context.Context, operation string)
	// On notify about (sub)task and progress processing operation
	On(events ...Resource)
	// Done is triggered as a Compose operation completed
	Done(operation string, success bool)
}
