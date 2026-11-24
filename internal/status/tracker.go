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

// Package status keeps the status of the operation Compose is running: the
// resources of the application and their state, and the reconciliation plan.
//
// It is what a progress display reads. The code that changes resources
// publishes there, and nothing reaches a display that is not in the status:
// whether the status is kept by the plan executor of the CLI, as here, or
// served by an engine that reconciles the application itself, the display is
// the same.
package status

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/docker/compose/v5/pkg/api"
)

// Tracker is the status of the operation being run. The code that operates on
// the application publishes to it, as a plan and as the states of its
// resources; a display consults it through api.StatusSource.
//
// It is also the api.EventProcessor of the backend. What is reported through
// On is folded into the status: an event about a container, a network or a
// volume is the new state of that resource, anything else is a task with its
// own progress. A sink given to New that is not a display drawing from the
// status (an SDK consumer, a plain or JSON writer) is handed those events
// unchanged, so what it receives does not depend on the status existing.
type Tracker struct {
	sink     api.EventProcessor
	renderer api.StatusRenderer // sink, when it draws from the status

	mu        sync.Mutex
	version   uint64
	resources []api.ResourceStatus
	byID      map[string]int
	plan      []api.PlanStep
	byStep    map[int]int
	tasks     []api.Resource
	byTask    map[taskKey]int
	notices   []string
	// depth is the number of operations in progress: Start without its Done.
	depth   int
	changed chan struct{}
}

type taskKey struct{ id, parent string }

var (
	_ api.EventProcessor = (*Tracker)(nil)
	_ api.StatusSource   = (*Tracker)(nil)
)

// New returns the status of an operation to be shown by sink, if any. A sink
// that is an api.StatusRenderer is attached to it.
func New(sink api.EventProcessor) *Tracker {
	t := &Tracker{
		sink:    sink,
		byID:    map[string]int{},
		byStep:  map[int]int{},
		byTask:  map[taskKey]int{},
		changed: make(chan struct{}),
	}
	if r, ok := sink.(api.StatusRenderer); ok {
		t.renderer = r
		r.Attach(t)
	}
	return t
}

// Start opens an operation.
func (t *Tracker) Start(ctx context.Context, operation string) {
	t.mu.Lock()
	t.depth++
	t.mu.Unlock()
	if t.sink != nil {
		t.sink.Start(ctx, operation)
	}
}

// Done closes an operation. When it is the outermost one, what the plan listed
// and never reached will not be: it is settled before the display is told, so
// that the last thing it draws is final.
func (t *Tracker) Done(operation string, success bool) {
	t.mu.Lock()
	if t.depth <= 1 && t.settle(success) {
		t.bump()
	}
	t.mu.Unlock()
	if t.sink != nil {
		t.sink.Done(operation, success)
	}
	t.mu.Lock()
	t.depth = max(t.depth-1, 0)
	t.mu.Unlock()
}

// settle ends what is still pending: resources and operations that no work is
// going to reach. It tells whether anything changed. The lock is held.
func (t *Tracker) settle(success bool) bool {
	message, level := "not reached", api.Warning
	if success {
		message, level = "nothing to do", api.Done
	}
	changed := false
	for i := range t.resources {
		if r := &t.resources[i]; r.Pending() {
			r.State, r.Level, r.Message = api.StateSkipped, level, message
			changed = true
		}
	}
	for i := range t.plan {
		if st := &t.plan[i]; st.State == api.StepPending {
			st.State, st.Message = api.StepSkipped, message
			changed = true
		}
	}
	return changed
}

// On folds events into the status. Events are handed to the sink as they come,
// unless it draws from the status and an operation is running: it reads them
// there.
func (t *Tracker) On(events ...api.Resource) {
	t.mu.Lock()
	changed := false
	for _, e := range events {
		if t.fold(e) {
			changed = true
		}
	}
	if changed {
		t.bump()
	}
	forward := t.sink != nil && (t.renderer == nil || t.depth == 0)
	t.mu.Unlock()
	if forward {
		t.sink.On(events...)
	}
}

// fold applies an event to the status, and tells whether it changed it. The
// lock is held.
func (t *Tracker) fold(e api.Resource) bool {
	if e.ID == api.ResourceCompose {
		if t.depth == 0 {
			// nothing is drawing the status: the sink prints it
			return false
		}
		msg := e.Details
		if msg == "" {
			msg = e.Text
		}
		t.notices = append(t.notices, msg)
		return true
	}
	if kind, name, ok := SplitID(e.ID); ok {
		return t.setResource(api.ResourceStatus{Kind: kind, Name: name, State: e.Text, Level: e.Status, Message: e.Details})
	}
	key := taskKey{id: e.ID, parent: e.ParentID}
	i, ok := t.byTask[key]
	if !ok {
		t.byTask[key] = len(t.tasks)
		t.tasks = append(t.tasks, e)
		return true
	}
	if t.tasks[i] == e {
		return false
	}
	t.tasks[i] = e
	return true
}

// SplitID tells the kind and the name of the resource an event ID designates
// ("Container app-web-1"), and whether it designates one.
func SplitID(id string) (kind, name string, ok bool) {
	for _, k := range []string{api.KindContainer, api.KindNetwork, api.KindVolume} {
		if name, found := strings.CutPrefix(id, k+" "); found && name != "" {
			return k, name, true
		}
	}
	return "", "", false
}

// setResource records the state of a resource, which keeps its place and its
// service if it was already known. It tells whether anything changed. The lock
// is held.
func (t *Tracker) setResource(r api.ResourceStatus) bool {
	id := r.ID()
	i, ok := t.byID[id]
	if !ok {
		t.byID[id] = len(t.resources)
		t.resources = append(t.resources, r)
		return true
	}
	if r.Service == "" {
		r.Service = t.resources[i].Service
	}
	if t.resources[i] == r {
		return false
	}
	t.resources[i] = r
	return true
}

// Plan publishes the reconciliation plan about to run: its operations, and the
// resources it is going to change, which are pending until the plan reaches
// them. A resource the plan names that is already known goes back to pending.
func (t *Tracker) Plan(steps []api.PlanStep, pending []api.ResourceStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.plan = slices.Clone(steps)
	t.byStep = make(map[int]int, len(steps))
	for i := range t.plan {
		t.plan[i].State = api.StepPending
		t.byStep[t.plan[i].ID] = i
	}
	for _, r := range pending {
		r.State, r.Level = api.StatePending, api.Working
		t.setResource(r)
	}
	t.bump()
}

// Step records the state of an operation of the plan.
func (t *Tracker) Step(id int, state api.StepState, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	i, ok := t.byStep[id]
	if !ok || (t.plan[i].State == state && t.plan[i].Message == message) {
		return
	}
	t.plan[i].State, t.plan[i].Message = state, message
	t.bump()
}

// bump marks a change. The lock is held.
func (t *Tracker) bump() {
	t.version++
	close(t.changed)
	t.changed = make(chan struct{})
}

// Status returns a snapshot of the status.
func (t *Tracker) Status() api.ProjectStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot()
}

// snapshot copies the status. The lock is held.
func (t *Tracker) snapshot() api.ProjectStatus {
	plan := slices.Clone(t.plan)
	for i := range plan {
		plan[i].DependsOn = slices.Clone(plan[i].DependsOn)
	}
	return api.ProjectStatus{
		Version:   t.version,
		Resources: slices.Clone(t.resources),
		Plan:      plan,
		Tasks:     slices.Clone(t.tasks),
		Notices:   slices.Clone(t.notices),
	}
}

// Wait returns a snapshot newer than the version given as soon as there is
// one. When the context ends first it returns the current snapshot and the
// error of the context.
func (t *Tracker) Wait(ctx context.Context, after uint64) (api.ProjectStatus, error) {
	for {
		t.mu.Lock()
		if t.version > after {
			s := t.snapshot()
			t.mu.Unlock()
			return s, nil
		}
		changed := t.changed
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return t.Status(), ctx.Err()
		case <-changed:
		}
	}
}
