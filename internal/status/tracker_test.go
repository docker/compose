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
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// recorder is a sink that is fed events.
type recorder struct {
	progress.Noop
	events []api.Resource
}

func (r *recorder) On(events ...api.Resource) { r.events = append(r.events, events...) }

// renderer is a sink that draws from the status.
type renderer struct {
	recorder
	source api.StatusSource
	starts []string
}

func (r *renderer) Attach(source api.StatusSource) { r.source = source }
func (r *renderer) Start(_ context.Context, op string) {
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

func TestWait(t *testing.T) {
	tr := New(nil)
	tr.On(progress.Creating("Volume data"))
	first := tr.Status()

	got := make(chan api.ProjectStatus, 1)
	go func() {
		st, err := tr.Wait(t.Context(), first.Version)
		assert.NilError(t, err)
		got <- st
	}()
	select {
	case <-got:
		t.Fatal("Wait returned before any change")
	case <-time.After(50 * time.Millisecond):
	}
	tr.On(progress.Created("Volume data"))
	select {
	case st := <-got:
		assert.Equal(t, st.Resources[0].State, api.StatusCreated)
		assert.Assert(t, st.Version > first.Version)
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after a change")
	}

	st, err := tr.Wait(t.Context(), 0)
	assert.NilError(t, err)
	assert.Equal(t, st.Version, tr.Status().Version, "an older version is answered at once")
}

func TestWaitEndsWithItsContext(t *testing.T) {
	tr := New(nil)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	st, err := tr.Wait(ctx, tr.Status().Version)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, st.Version, tr.Status().Version)
}

func TestPlanBringsAKnownResourceBackToPending(t *testing.T) {
	tr := New(nil)
	tr.On(progress.Created("Container app-web-1"))
	tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "app-web-1", Message: "recreate: config hash changed"}})
	r := tr.Status().Resources[0]
	assert.Equal(t, r.State, api.StatePending)
	assert.Equal(t, r.Message, "recreate: config hash changed")
}
