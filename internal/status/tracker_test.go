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

package status

import (
	"context"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// recorder is a sink that is fed events. Like a display, it guards itself.
type recorder struct {
	progress.Noop
	mu     sync.Mutex
	events []api.Resource
}

func (r *recorder) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

// renderer is a sink that draws from the status.
type renderer struct {
	recorder
	source api.StatusSource
	starts []string
}

func (r *renderer) Attach(source api.StatusSource) { r.source = source }
func (r *renderer) Start(_ context.Context, op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, op)
}

func TestEventsOfResourcesBecomeTheirState(t *testing.T) {
	tr := New(nil)
	tr.On(progress.Creating("Network app_default"), progress.Creating("Container app-web-1"))
	tr.On(progress.Created("Network app_default"))

	st := tr.Status()
	assert.DeepEqual(t, st.Resources, []api.ResourceStatus{
		{Kind: api.KindNetwork, Name: "app_default", State: api.StatusCreated, Level: api.Done},
		{Kind: api.KindContainer, Name: "app-web-1", State: api.StatusCreating, Level: api.Working},
	})
	assert.Equal(t, len(st.Tasks), 0)
}

func TestOtherEventsAreTasksKeptPerParent(t *testing.T) {
	tr := New(nil)
	tr.On(
		api.Resource{ID: "Image a", Status: api.Working, Text: api.StatusPulling},
		api.Resource{ID: "layer1", ParentID: "Image a", Status: api.Working, Text: "Downloading", Percent: 10},
		api.Resource{ID: "layer1", ParentID: "Image b", Status: api.Working, Text: "Downloading", Percent: 20},
		api.Resource{ID: "layer1", ParentID: "Image a", Status: api.Done, Text: api.StatusDownloadComplete},
	)
	st := tr.Status()
	assert.Equal(t, len(st.Resources), 0)
	assert.DeepEqual(t, st.Tasks, []api.Resource{
		{ID: "Image a", Status: api.Working, Text: api.StatusPulling},
		{ID: "layer1", ParentID: "Image a", Status: api.Done, Text: api.StatusDownloadComplete},
		{ID: "layer1", ParentID: "Image b", Status: api.Working, Text: "Downloading", Percent: 20},
	})
}

func TestPlanPublishesPendingResourcesThenEventsMoveThem(t *testing.T) {
	tr := New(nil)
	tr.Plan(
		[]api.PlanStep{
			{ID: 1, Operation: "create_network", Resource: "Network app_default"},
			{ID: 2, Operation: "create_container", Resource: "Container app-web-1", Service: "web", DependsOn: []int{1}},
		},
		[]api.ResourceStatus{
			{Kind: api.KindNetwork, Name: "app_default", Message: "create"},
			{Kind: api.KindContainer, Name: "app-web-1", Service: "web", Message: "create"},
		},
	)
	st := tr.Status()
	assert.Equal(t, st.Resources[0].State, api.StatePending)
	assert.Assert(t, st.Resources[0].Pending())
	assert.Equal(t, st.Plan[1].State, api.StepPending)

	tr.On(progress.Creating("Container app-web-1"))
	tr.Step(2, api.StepRunning, "")
	st = tr.Status()
	assert.Equal(t, st.Resources[1].State, api.StatusCreating)
	assert.Equal(t, st.Resources[1].Service, "web", "the service survives the events that do not name it")
	assert.Equal(t, st.Resources[1].Message, "")
	assert.Equal(t, st.Plan[1].State, api.StepRunning)
	assert.DeepEqual(t, st.Plan[1].DependsOn, []int{1})
}

func TestStatusIsASnapshot(t *testing.T) {
	tr := New(nil)
	tr.Plan([]api.PlanStep{{ID: 1, DependsOn: []int{7}}}, nil)
	st := tr.Status()
	st.Plan[0].DependsOn[0] = 99
	st.Plan[0].State = api.StepFailed
	assert.DeepEqual(t, tr.Status().Plan[0].DependsOn, []int{7})
	assert.Equal(t, tr.Status().Plan[0].State, api.StepPending)
}

func TestVersionGrowsOnlyWithChanges(t *testing.T) {
	tr := New(nil)
	tr.On(progress.Creating("Volume data"))
	v := tr.Status().Version
	tr.On(progress.Creating("Volume data"))
	assert.Equal(t, tr.Status().Version, v, "the same state again is no change")
	tr.On(progress.Created("Volume data"))
	assert.Assert(t, tr.Status().Version > v)

	tr.Plan([]api.PlanStep{{ID: 1}}, nil)
	v = tr.Status().Version
	tr.Step(1, api.StepRunning, "")
	assert.Equal(t, tr.Status().Version, v+1)
	tr.Step(1, api.StepRunning, "")
	assert.Equal(t, tr.Status().Version, v+1)
	tr.Step(42, api.StepRunning, "")
	assert.Equal(t, tr.Status().Version, v+1, "an unknown step changes nothing")
}

func TestSinkReceivesEventsUnchanged(t *testing.T) {
	sink := &recorder{}
	tr := New(sink)
	tr.Start(t.Context(), "up")
	events := []api.Resource{
		progress.Creating("Container app-web-1"),
		progress.Creating("Container app-web-1"),
		{ID: api.ResourceCompose, Details: "stopping"},
		progress.Created("Container app-web-1"),
	}
	tr.On(events...)
	tr.Done("up", true)
	assert.DeepEqual(t, sink.events, events)
}

func TestRendererReadsTheStatusDuringAnOperation(t *testing.T) {
	sink := &renderer{}
	tr := New(sink)
	assert.Assert(t, sink.source == api.StatusSource(tr), "the renderer is attached")

	tr.On(progress.Creating("Network early"))
	assert.Equal(t, len(sink.events), 1, "outside an operation, the sink is told")

	tr.Start(t.Context(), "up")
	tr.On(progress.Creating("Container app-web-1"), api.Resource{ID: api.ResourceCompose, Details: "Gracefully Stopping..."})
	assert.Equal(t, len(sink.events), 1, "during an operation, the sink reads the status")
	st := sink.source.Status()
	assert.Equal(t, st.Resources[1].State, api.StatusCreating)
	assert.DeepEqual(t, st.Notices, []string{"Gracefully Stopping..."})
	tr.Done("up", true)

	tr.On(progress.Created("Container app-web-1"))
	assert.Equal(t, len(sink.events), 2, "outside an operation again")
	assert.DeepEqual(t, sink.starts, []string{"up"})
}

func TestNoticeOutsideAnOperationIsLeftToTheSink(t *testing.T) {
	sink := &renderer{}
	tr := New(sink)
	tr.On(api.Resource{ID: api.ResourceCompose, Details: "hello"})
	assert.Equal(t, len(sink.events), 1)
	assert.Equal(t, len(tr.Status().Notices), 0)
}

func TestVersionIsThatOfTheStatus(t *testing.T) {
	tr := New(nil)
	assert.Equal(t, tr.Version(), tr.Status().Version)
	tr.On(progress.Creating("Volume data"))
	assert.Equal(t, tr.Version(), tr.Status().Version)
	assert.Assert(t, tr.Version() > 0)
}

func TestPlanBringsAKnownResourceBackToPending(t *testing.T) {
	tr := New(nil)
	tr.On(progress.Created("Container app-web-1"))
	tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "app-web-1", Message: "recreate: config hash changed"}})
	r := tr.Status().Resources[0]
	assert.Equal(t, r.State, api.StatePending)
	assert.Equal(t, r.Message, "recreate: config hash changed")
}

func TestWhatThePlanNeverReachedIsSettledWhenTheOperationEnds(t *testing.T) {
	plan := func(tr *Tracker) {
		tr.Plan(
			[]api.PlanStep{{ID: 1}, {ID: 2}},
			[]api.ResourceStatus{{Kind: api.KindContainer, Name: "a", Message: "create"}, {Kind: api.KindContainer, Name: "b", Message: "create"}},
		)
		tr.On(progress.Created("Container a"))
		tr.Step(1, api.StepSucceeded, "")
	}

	t.Run("failure", func(t *testing.T) {
		tr := New(nil)
		tr.Start(t.Context(), "up")
		plan(tr)
		tr.Done("up", false)
		st := tr.Status()
		assert.Equal(t, st.Resources[0].State, api.StatusCreated)
		assert.DeepEqual(t, st.Resources[1], api.ResourceStatus{Kind: api.KindContainer, Name: "b", State: api.StateSkipped, Level: api.Warning, Message: "not reached"})
		assert.Equal(t, st.Plan[0].State, api.StepSucceeded)
		assert.Equal(t, st.Plan[1].State, api.StepSkipped)
		assert.Equal(t, st.Plan[1].Message, "not reached")
	})

	t.Run("success", func(t *testing.T) {
		tr := New(nil)
		tr.Start(t.Context(), "down")
		plan(tr)
		tr.Done("down", true)
		r := tr.Status().Resources[1]
		assert.Equal(t, r.State, api.StateSkipped)
		assert.Equal(t, r.Level, api.Done)
		assert.Equal(t, r.Message, "nothing to do")
	})

	t.Run("a nested operation ending settles nothing", func(t *testing.T) {
		tr := New(nil)
		tr.Start(t.Context(), "up")
		tr.Start(t.Context(), "start")
		plan(tr)
		tr.Done("start", true)
		assert.Assert(t, tr.Status().Resources[1].Pending())
		tr.Done("up", true)
		assert.Equal(t, tr.Status().Resources[1].State, api.StateSkipped)
	})

	t.Run("a Done that matches no Start settles nothing", func(t *testing.T) {
		tr := New(nil)
		tr.Start(t.Context(), "up")
		plan(tr)
		tr.Done("up", true)
		tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "c", Message: "create"}})
		tr.Done("up", true)
		assert.Assert(t, tr.Status().Resources[2].Pending())
	})

	t.Run("the display is told after the settling", func(t *testing.T) {
		sink := &renderer{}
		tr := New(sink)
		tr.Start(t.Context(), "up")
		plan(tr)
		tr.Done("up", true)
		assert.Equal(t, sink.source.Status().Resources[1].State, api.StateSkipped)
	})
}

// A display keeps the operation that runs another in progress when the inner one
// ends: what is reported meanwhile is read from the status, and not handed to a
// display that would print it as a line outside of the operation.
func TestAnEventAfterAnInnerDoneStillGoesThroughTheStatus(t *testing.T) {
	sink := &renderer{}
	tr := New(sink)
	tr.Start(t.Context(), "up")
	tr.Start(t.Context(), "build") // a nested operation
	tr.Done("build", true)

	tr.On(progress.Created("Container app-web-1"))
	tr.On(api.Resource{ID: api.ResourceCompose, Details: "Gracefully Stopping..."})
	assert.Equal(t, len(sink.events), 0, "the display of the outer operation reads the status")
	st := tr.Status()
	assert.Equal(t, st.Resources[0].State, api.StatusCreated)
	assert.DeepEqual(t, st.Notices, []string{"Gracefully Stopping..."})

	// once the last operation is over nothing reads the status any more
	tr.Done("up", true)
	tr.On(progress.Created("Container late-1"))
	assert.Equal(t, len(sink.events), 1)
}

func TestDeferKeepsWhatRunsAfterTheOperationPending(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.Plan(
		[]api.PlanStep{
			{ID: 1, Resource: "Container app-db-1"},
			{ID: 2, Resource: "Container app-db-1"},
			{ID: 3, Resource: "Container app-web-1"},
		},
		[]api.ResourceStatus{
			{Kind: api.KindContainer, Name: "app-db-1", Message: "create, start"},
			{Kind: api.KindContainer, Name: "app-web-1", Message: "start"},
		},
	)
	tr.Step(1, api.StepSucceeded, "")
	tr.Defer(2, 3) // the start phase
	tr.Done("up", true)

	st := tr.Status()
	assert.Equal(t, st.Plan[1].State, api.StepPending)
	assert.Equal(t, st.Plan[2].State, api.StepPending)
	assert.Assert(t, st.Resources[1].Pending(), "only the deferred operation changes app-web-1")

	// it runs on its own, outside of any operation, and is then settled by its outcome
	tr.Step(3, api.StepSucceeded, "")
	assert.Equal(t, tr.Status().Plan[2].State, api.StepSucceeded)
}

func TestDeferSettlesTheResourcesWhateverIsDeferred(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.Plan(
		[]api.PlanStep{{ID: 1, Resource: "Container a"}, {ID: 2, Resource: "Container a"}},
		[]api.ResourceStatus{{Kind: api.KindContainer, Name: "a", Message: "create, start"}},
	)
	tr.Defer(2)
	tr.Done("up", true)
	// a is also changed by an operation that ran in the operation: only the
	// operations not deferred are the ones the end of it answers for
	assert.Equal(t, tr.Status().Resources[0].State, api.StateSkipped)

	// and a deferral does not outlive the operation it was made for
	tr.Start(t.Context(), "up")
	tr.Plan([]api.PlanStep{{ID: 1}}, []api.ResourceStatus{{Kind: api.KindContainer, Name: "b", Message: "create"}})
	tr.Done("up", false)
	assert.Equal(t, tr.Status().Plan[0].State, api.StepSkipped)
}

func TestAResourceKeepsWhatThePlanDoesToIt(t *testing.T) {
	tr := New(nil)
	tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "app-web-1", Message: "create, start", Action: "create, start"}})
	tr.On(progress.Creating("Container app-web-1"))
	tr.On(progress.New("Container app-web-1", api.Done, api.StatusStarted))
	r := tr.Status().Resources[0]
	assert.Equal(t, r.State, api.StatusStarted)
	assert.Equal(t, r.Action, "create, start", "the action outlives the pending state")
}

func TestANoticeIsItsDetailsAsTheSinkPrintsThem(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.On(api.Resource{ID: api.ResourceCompose, Text: "ignored", Details: "Gracefully Stopping..."})
	assert.DeepEqual(t, tr.Status().Notices, []string{"Gracefully Stopping..."})
	tr.Done("up", true)
}

func TestAnEventRightAfterDoneIsNotLost(t *testing.T) {
	sink := &renderer{}
	tr := New(sink)
	tr.Start(t.Context(), "up")
	tr.Done("up", true)
	tr.On(progress.Created("Container late-1"))
	assert.DeepEqual(t, sink.events, []api.Resource{progress.Created("Container late-1")})
}

func TestTasksOfAnOperationAreForgottenByTheNextOne(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "build")
	tr.On(api.Resource{ID: "Image web", Status: api.Working, Text: api.StatusBuilding})
	tr.Done("build", false) // canceled: no end was ever reported
	assert.Equal(t, len(tr.Status().Tasks), 1)

	v := tr.Status().Version
	tr.Start(t.Context(), "up")
	st := tr.Status()
	assert.Equal(t, len(st.Tasks), 0)
	assert.Assert(t, st.Version > v)

	// a nested operation does not forget the tasks of the one it is part of
	tr.On(api.Resource{ID: "Image web", Status: api.Done, Text: api.StatusPulled})
	tr.Start(t.Context(), "nested")
	assert.Equal(t, len(tr.Status().Tasks), 1)
}

func TestTrackerIsSafeForConcurrentUse(t *testing.T) {
	sink := &renderer{}
	tr := New(sink)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() { // a display reading
		defer close(done)
		var seen uint64
		for ctx.Err() == nil {
			if v := tr.Version(); v != seen {
				seen = tr.Status().Version
			}
			time.Sleep(time.Microsecond)
		}
	}()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "Container c" + string(rune('a'+i))
			for range 200 {
				tr.Start(ctx, "up")
				tr.Plan([]api.PlanStep{{ID: i}}, []api.ResourceStatus{{Kind: api.KindContainer, Name: name[len("Container "):]}})
				tr.On(progress.Creating(name), api.Resource{ID: "Image x", ParentID: name, Status: api.Working})
				tr.Step(i, api.StepRunning, "")
				_ = tr.Status()
				tr.On(progress.Created(name))
				tr.Done("up", true)
			}
		}()
	}
	wg.Wait()
	cancel()
	<-done
}

// A start phase that never runs (the attach failed, Ctrl-C) leaves nothing
// pending for ever: the next operation settles it.
func TestWhatWasDeferredAndNeverRanIsSettledByTheNextOperation(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.Plan(
		[]api.PlanStep{{ID: 1, Resource: "Container a"}, {ID: 2, Resource: "Container b"}},
		[]api.ResourceStatus{{Kind: api.KindContainer, Name: "a", Message: "create"}, {Kind: api.KindContainer, Name: "b", Message: "start"}},
	)
	tr.Step(1, api.StepSucceeded, "")
	tr.Defer(2)
	tr.Done("up", true)
	assert.Equal(t, tr.Status().Plan[1].State, api.StepPending)

	before := tr.Version()
	tr.Start(t.Context(), "down")
	st := tr.Status()
	assert.Assert(t, tr.Version() > before)
	assert.Equal(t, st.Plan[1].State, api.StepSkipped)
	assert.Equal(t, st.Plan[1].Message, "not reached")
	assert.Equal(t, st.Resources[1].State, api.StateSkipped)
	assert.Equal(t, st.Resources[1].Level, api.Warning)
}

func TestAPlanReplacesWhatWasDeferred(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.Plan([]api.PlanStep{{ID: 1}}, nil)
	tr.Defer(1)
	tr.Plan([]api.PlanStep{{ID: 1}}, nil) // a new plan: nothing of the old one is deferred
	tr.Done("up", true)
	assert.Equal(t, tr.Status().Plan[0].State, api.StepSkipped)
}

// What the plan did to a resource in an operation says nothing of what a later
// operation does to it.
func TestTheActionOfAResourceDoesNotOutliveItsOperation(t *testing.T) {
	tr := New(nil)
	tr.Start(t.Context(), "up")
	tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "app-web-1", Message: "create, start", Action: "create, start"}})
	tr.On(progress.New("Container app-web-1", api.Done, api.StatusStarted))
	tr.Done("up", true)
	assert.Equal(t, tr.Status().Resources[0].Action, "create, start", "still there once the operation is over: a start phase may follow")

	tr.Start(t.Context(), "restart")
	tr.On(progress.New("Container app-web-1", api.Done, api.StatusStarted))
	assert.Equal(t, tr.Status().Resources[0].Action, "")
	tr.Done("restart", true)
}
