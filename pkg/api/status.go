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

import "context"

// Resource kinds a ProjectStatus reports. A kind is also the first word of the
// ID of the matching Resource events ("Container myapp-web-1").
const (
	KindContainer = "Container"
	KindNetwork   = "Network"
	KindVolume    = "Volume"
)

// StatePending is the state of a resource the reconciliation plan is going to
// change and has not reached yet.
const StatePending = "Pending"

// ProjectStatus is what a progress display consults to show an application: the
// resources it is made of and their state, and the plan that is being run to
// converge them. It is a snapshot; Version grows with every change, so two
// snapshots of the same Version are equal.
//
// It is the status a reconciliation exposes, whoever runs it: the plan executor
// of the CLI today, a Compose extension of the engine through its Project
// resource. A display written against it works the same for both.
type ProjectStatus struct {
	// Version is incremented by every change of the status.
	Version uint64
	// Resources are the containers, networks and volumes of the application
	// the operation concerns, in the order the plan reaches them.
	Resources []ResourceStatus
	// Plan is the list of operations of the current reconciliation, empty when
	// nothing has been planned (an operation that does not reconcile).
	Plan []PlanStep
	// Tasks are the activities that belong to no resource of the application:
	// pulling and building images, pushing, copying files. They carry their
	// own progress.
	Tasks []Resource
	// Notices are messages for the user that are not tied to a resource, in
	// the order they were raised.
	Notices []string
}

// ResourceStatus is the state of one resource of the application.
type ResourceStatus struct {
	Kind    string
	Name    string
	Service string
	// State is what the resource is doing or has become, in the words of the
	// Status* constants ("Creating", "Started"), or StatePending.
	State string
	// Level tells whether State is under way (Working), reached (Done), or
	// ended in a warning or an error.
	Level EventStatus
	// Message gives the reason of a pending action or the detail of a warning
	// or an error.
	Message string
}

// ID is the identifier a Resource event of this resource goes by.
func (r ResourceStatus) ID() string { return r.Kind + " " + r.Name }

// Pending tells whether the plan has yet to reach the resource.
func (r ResourceStatus) Pending() bool { return r.State == StatePending }

// StepState is the state of an operation of the plan.
type StepState string

// The states of an operation of the plan.
const (
	StepPending   StepState = "pending"
	StepRunning   StepState = "running"
	StepSucceeded StepState = "succeeded"
	StepFailed    StepState = "failed"
	// StepSkipped is an operation that did not run, or did not complete,
	// because another one failed or the operation was interrupted.
	StepSkipped StepState = "skipped"
)

// PlanStep is one operation of the reconciliation plan.
type PlanStep struct {
	ID int
	// Operation names what is done: "create_container", "remove_network"...
	Operation string
	// Resource is the ID of the resource the operation changes, if it has one.
	Resource string
	Service  string
	// Reason says why the operation is needed ("config hash changed").
	Reason    string
	DependsOn []int
	State     StepState
	Message   string
}

// StatusSource gives access to the status of the operation being run.
type StatusSource interface {
	// Status returns the current snapshot.
	Status() ProjectStatus
	// Wait returns a snapshot more recent than the Version given, as soon as
	// there is one, or the current one when the context ends.
	Wait(ctx context.Context, after uint64) (ProjectStatus, error)
}

// StatusRenderer is a progress display that draws from the status of the
// operation instead of being fed its events. Compose attaches it to the status
// before any operation runs, and only calls On for the events that are raised
// outside of an operation, which have no status to be read from.
type StatusRenderer interface {
	EventProcessor
	// Attach gives the display the status it draws from.
	Attach(source StatusSource)
}
