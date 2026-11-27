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

package display

import (
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/status"
	"github.com/docker/compose/v5/pkg/api"
)

// fakeSource is a status the test changes by hand.
type fakeSource struct{ st api.ProjectStatus }

func (f *fakeSource) Status() api.ProjectStatus { return f.st }
func (f *fakeSource) Version() uint64           { return f.st.Version }

func (f *fakeSource) set(resources ...api.ResourceStatus) {
	f.st.Version++
	f.st.Resources = resources
}

// frame is what the writer would paint, without colors.
func frame(w *termWriter) []string {
	_, height := w.size()
	lines := layoutFrame(&w.tree, w.operation, layoutOpts{width: 100, height: height, now: w.now()})
	for i := range lines {
		lines[i] = strings.TrimRight(stripAnsi(lines[i]), " ")
	}
	return lines
}

func pendingRow(kind, name, message string) api.ResourceStatus {
	action, _, _ := strings.Cut(message, ":")
	return api.ResourceStatus{Kind: kind, Name: name, State: api.StatePending, Level: api.Working, Message: message, Action: action}
}

func TestSyncShowsThePlanThenTheStateOfEachResource(t *testing.T) {
	w, _, clock := newTermWriter(100, 20)
	w.operation = "up"
	src := &fakeSource{}
	w.Attach(src)

	src.set(
		pendingRow(api.KindNetwork, "app_default", "create"),
		pendingRow(api.KindContainer, "app-web-1", "recreate: config hash diverged"),
	)
	w.sync()
	lines := frame(w)
	assert.Equal(t, lines[0], "[+] up 0/2")
	assert.Assert(t, strings.Contains(lines[1], "Network app_default"), lines[1])
	assert.Assert(t, strings.Contains(lines[1], "Pending create"), lines[1])
	assert.Assert(t, strings.Contains(lines[2], "Container app-web-1"), lines[2])
	assert.Assert(t, strings.Contains(lines[2], "Pending recreate: config hash diverged"), lines[2])
	assert.Assert(t, strings.HasPrefix(lines[1], " ·"), "a pending row has no spinner: %q", lines[1])
	assert.Assert(t, !strings.Contains(lines[1], "0.0s"), "and no timer: %q", lines[1])

	clock.Advance(2 * time.Second)
	src.set(
		api.ResourceStatus{Kind: api.KindNetwork, Name: "app_default", State: api.StatusCreated, Level: api.Done},
		api.ResourceStatus{Kind: api.KindContainer, Name: "app-web-1", State: "Recreate", Level: api.Working},
	)
	w.sync()
	lines = frame(w)
	assert.Equal(t, lines[0], "[+] up 1/2")
	assert.Assert(t, strings.Contains(lines[1], "✔"), lines[1])
	assert.Assert(t, strings.Contains(lines[1], "Created"), lines[1])
	assert.Assert(t, strings.Contains(lines[2], "Recreate"), lines[2])
	assert.Equal(t, len(w.tree.roots), 2, "the rows of the plan are updated in place")
	assert.Assert(t, strings.Contains(lines[2], "0.0s"), "the work is timed from when it starts: %q", lines[2])
}

func TestSyncIsANoOpWithoutANewVersion(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)
	src.set(pendingRow(api.KindVolume, "data", "create"))
	w.sync()
	// tamper with the tree: a second sync of the same version must not redo it
	w.tree.nodes["Volume data"].text = "kept"
	w.sync()
	assert.Equal(t, w.tree.nodes["Volume data"].text, "kept")
}

func TestSyncWithoutAStatusDoesNothing(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 0)
}

func TestAttachedUpDoesNotPaintOverTheLogs(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	w.operation = "up"
	src := &fakeSource{}
	w.Attach(src)

	src.set(
		pendingRow(api.KindContainer, "app-db-1", "start"),
		pendingRow(api.KindContainer, "app-web-1", "create, start"),
	)
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 1, "starting an existing container is not announced")
	assert.Assert(t, w.tree.nodes["Container app-web-1"] != nil)

	src.set(
		api.ResourceStatus{Kind: api.KindContainer, Name: "app-db-1", State: api.StatusStarted, Level: api.Done},
		api.ResourceStatus{Kind: api.KindContainer, Name: "app-web-1", State: api.StatusCreated, Level: api.Done},
	)
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 1)
	assert.Equal(t, w.tree.nodes["Container app-web-1"].text, api.StatusCreated)

	// the same status, shown for a command that is about starting containers
	w2, _, _ := newTermWriter(100, 20)
	w2.operation = "start"
	w2.Attach(src)
	w2.sync()
	assert.Equal(t, len(w2.tree.nodes), 2)

	// and for a detached up
	w3, _, _ := newTermWriter(100, 20)
	w3.operation = "up"
	w3.detached = true
	w3.Attach(src)
	w3.sync()
	assert.Equal(t, len(w3.tree.nodes), 2)
}

func TestSyncPrintsEachNoticeOnce(t *testing.T) {
	w, buf, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)
	src.st.Version = 1
	src.st.Notices = []string{"Gracefully Stopping..."}
	w.sync()
	w.sync()
	src.st.Version = 2
	src.st.Notices = append(src.st.Notices, "Aborting on container exit...")
	w.sync()
	out := stripAnsi(buf.String())
	assert.Equal(t, strings.Count(out, "Gracefully Stopping..."), 1)
	assert.Equal(t, strings.Count(out, "Aborting on container exit..."), 1)
}

func TestSyncStaysSilentWhileAnImageIsBuilt(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)

	src.st.Version = 1
	src.st.Tasks = []api.Resource{{ID: "Image web", Status: api.Working, Text: api.StatusBuilding}}
	w.sync()
	assert.Assert(t, w.suspended)

	src.st.Version = 2
	src.st.Tasks = []api.Resource{{ID: "Image web", Status: api.Done, Text: api.StatusBuilt}}
	w.sync()
	assert.Assert(t, !w.suspended)
	assert.Equal(t, w.tree.nodes["Image web"].text, api.StatusBuilt)
}

// The tracker of the backend and the writer meet as they do in a command: the
// writer is attached by the tracker and reads what the tracker keeps.
func TestWriterDrawsFromTheTrackerOfTheBackend(t *testing.T) {
	var out strings.Builder
	w := newTerm(&out, &out, true)
	w.size = func() (int, int) { return 100, 20 }
	tr := status.New(w)
	assert.Assert(t, w.source != nil)

	tr.Start(t.Context(), "up")
	tr.Plan(
		[]api.PlanStep{{ID: 1, Operation: "create_network", Resource: "Network app_default"}},
		[]api.ResourceStatus{{Kind: api.KindNetwork, Name: "app_default", Message: "create"}},
	)
	tr.On(api.Resource{ID: "Network app_default", Status: api.Done, Text: api.StatusCreated})
	tr.Done("up", true)

	assert.Assert(t, strings.Contains(stripAnsi(out.String()), "Network app_default"), out.String())
	assert.Assert(t, strings.Contains(stripAnsi(out.String()), "Created"), out.String())
	assert.Equal(t, w.tree.nodes["Network app_default"].text, api.StatusCreated)
}

func TestStartLooksAtTheStatusAgain(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)

	src.set(pendingRow(api.KindContainer, "app-db-1", "start"))
	w.operation = "up"
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 0, "held back while attached")

	// the status did not change, but the operation did: starting is what it is about
	w.Start(t.Context(), "start")
	defer w.Done("start", true)
	w.mu.Lock()
	w.sync()
	w.mu.Unlock()
	assert.Equal(t, len(w.tree.nodes), 1)
}

func TestWhatWasNeverShownIsNotShownWhenSettled(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	w.operation = "up"
	src := &fakeSource{}
	w.Attach(src)

	src.set(pendingRow(api.KindContainer, "app-db-1", "start"), pendingRow(api.KindContainer, "app-web-1", "create"))
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 1)

	src.set(
		api.ResourceStatus{Kind: api.KindContainer, Name: "app-db-1", State: api.StateSkipped, Level: api.Done, Message: "nothing to do"},
		api.ResourceStatus{Kind: api.KindContainer, Name: "app-web-1", State: api.StateSkipped, Level: api.Warning, Message: "not reached"},
	)
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 1, "the held back row stays out")
	assert.Equal(t, w.tree.nodes["Container app-web-1"].text, api.StateSkipped, "the one that was shown is settled")
}

// A container the plan creates goes from created to started between two looks
// at the status: it is shown all the same, since what the plan does to it is
// what decides, and not the state it happens to be sampled in.
func TestAttachedOperationShowsWhatItCreatedAndStarted(t *testing.T) {
	var out strings.Builder
	w := newTerm(&out, &out, false)
	w.size = func() (int, int) { return 100, 20 }
	tr := status.New(w)

	tr.Start(t.Context(), "scale")
	tr.Plan(nil, []api.ResourceStatus{
		{Kind: api.KindContainer, Name: "app-web-2", Message: "create, start", Action: "create, start"},
		{Kind: api.KindContainer, Name: "app-db-1", Message: "start", Action: "start"},
	})
	for _, name := range []string{"app-web-2", "app-db-1"} {
		id := "Container " + name
		if name == "app-web-2" {
			tr.On(api.Resource{ID: id, Status: api.Working, Text: api.StatusCreating})
			tr.On(api.Resource{ID: id, Status: api.Done, Text: api.StatusCreated})
		}
		tr.On(api.Resource{ID: id, Status: api.Working, Text: api.StatusStarting})
		tr.On(api.Resource{ID: id, Status: api.Done, Text: api.StatusStarted})
	}
	tr.Done("scale", true)

	created := w.tree.nodes["Container app-web-2"]
	assert.Assert(t, created != nil, "a container the plan creates is shown")
	assert.Assert(t, created.completed() && created.text == api.StatusStarted, "ends as %q", created.text)
	assert.Assert(t, w.tree.nodes["Container app-db-1"] == nil, "one that is only started is not")
}

// run and up chain operations: when the inner one ends the outer one goes on,
// and it is its final state that is left on screen.
func TestTheOuterOperationIsDrawnToTheEndWhenAnInnerOneEnds(t *testing.T) {
	var out strings.Builder
	w := newTerm(&out, &out, true)
	w.size = func() (int, int) { return 100, 20 }
	tr := status.New(w)

	tr.Start(t.Context(), "up")
	tr.Start(t.Context(), "pull")
	tr.On(api.Resource{ID: "Image web", Status: api.Working, Text: api.StatusPulling})
	tr.Done("pull", true)
	assert.Equal(t, w.operation, "up", "the display is about the outer operation again")
	assert.Assert(t, w.stopTicks != nil, "and it goes on being refreshed")

	tr.Plan(nil, []api.ResourceStatus{{Kind: api.KindContainer, Name: "app-web-1", Message: "create", Action: "create"}})
	tr.On(api.Resource{ID: "Container app-web-1", Status: api.Done, Text: api.StatusCreated})
	tr.Done("up", true)

	assert.Equal(t, w.operation, "")
	assert.Assert(t, w.stopTicks == nil)
	final := stripAnsi(out.String())
	assert.Assert(t, strings.Contains(final, "Container app-web-1"), final)
	assert.Assert(t, strings.Contains(final, "Created"), "the settled state of the outer operation is painted: %s", final)
	assert.Assert(t, !strings.Contains(final, "Container app-web-1 Created \n"), "and not printed as a line outside of the block")
}

func TestAttachingAnotherStatusForgetsWhatWasDrawnFromTheFirst(t *testing.T) {
	w, buf, _ := newTermWriter(100, 20)
	first := &fakeSource{}
	first.st.Version, first.st.Notices = 1, []string{"first notice"}
	w.Attach(first)
	w.sync()

	second := &fakeSource{}
	second.st.Version, second.st.Notices = 1, []string{"second notice"}
	w.Attach(second)
	w.sync()

	out := stripAnsi(buf.String())
	assert.Equal(t, strings.Count(out, "first notice"), 1)
	assert.Equal(t, strings.Count(out, "second notice"), 1, "same version, same number of notices: still told")
}

// A build that begins and ends between two looks at the status was never seen
// working: its output is on the screen below the block, which must start anew.
func TestABuildTooShortToBeSeenLeavesTheBlockToBeRepaintedAnew(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)
	w.scr.prev = []string{"a row of the block", "another"}

	src.st.Version = 1
	src.st.Tasks = []api.Resource{{ID: "Image web", Status: api.Done, Text: api.StatusBuilt}}
	w.sync()
	assert.Assert(t, w.scr.prev == nil, "the cursor is not moved back over lines that are not the block's")
	assert.Assert(t, !w.suspended)
}

func TestSyncDoesNotReadTheStatusWhenItsVersionIsTheOneDrawn(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &countingSource{}
	w.Attach(src)
	src.st.Version = 1
	w.sync()
	w.sync()
	w.sync()
	assert.Equal(t, src.statuses, 1, "the snapshot is only built for a new version")
}

type countingSource struct {
	fakeSource
	statuses int
}

func (c *countingSource) Status() api.ProjectStatus {
	c.statuses++
	return c.st
}

// An operation that is run by another is not what decides whether container
// output shares the terminal: that is the one the user ran.
func TestAnOperationRunByAnotherDoesNotShowWhatTheOuterOneHides(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)

	w.Start(t.Context(), "up")
	w.Start(t.Context(), "start") // run by up
	src.set(pendingRow(api.KindContainer, "app-db-1", "start"))
	w.sync()
	assert.Equal(t, len(w.tree.nodes), 0, "up streams the logs of the containers it starts")
	w.Done("start", true)
	w.Done("up", true)

	w2, _, _ := newTermWriter(100, 20)
	w2.Attach(src)
	w2.Start(t.Context(), "start")
	w2.sync()
	assert.Equal(t, len(w2.tree.nodes), 1, "start is about starting")
	w2.Done("start", true)
}

func TestADoneWithNoStartIsHarmless(t *testing.T) {
	w, _, _ := newTermWriter(100, 20)
	src := &fakeSource{}
	w.Attach(src)
	src.set(pendingRow(api.KindVolume, "data", "create"))

	w.Done("up", true)
	assert.Equal(t, w.operation, "")
	assert.Equal(t, len(w.operations), 0)

	// and two Done for one Start do not go below nothing
	w.Start(t.Context(), "up")
	w.Done("up", true)
	w.Done("up", true)
	assert.Equal(t, len(w.operations), 0)
}
