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
	"time"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// node is the rendered state of a single task, fed exclusively by
// taskTree.apply. It holds no rendering concern: no spinner, no strings
// destined for the screen, no clock reads.
type node struct {
	id        string
	anchor    string // first parent seen; only events carrying it update the node
	parents   utils.Set[string]
	text      string
	details   string
	status    api.EventStatus
	current   int64
	total     int64
	percent   int
	startedAt time.Time
	endedAt   time.Time
}

func (n *node) completed() bool {
	switch n.status {
	case api.Done, api.Error, api.Warning:
		return true
	default:
		return false
	}
}

// taskTree stores tasks in arrival order and resolves parent/child links.
// It is a plain data structure: all mutation goes through apply, all time is
// injected, so its behavior is exhaustively testable without a terminal.
type taskTree struct {
	order []string
	nodes map[string]*node
}

func newTaskTree() taskTree {
	return taskTree{nodes: map[string]*node{}}
}

// apply is the single state transition of the model.
func (t *taskTree) apply(e api.Resource, now time.Time) {
	n, ok := t.nodes[e.ID]
	if !ok {
		n = &node{
			id:        e.ID,
			anchor:    e.ParentID,
			parents:   utils.NewSet[string](),
			startedAt: now,
		}
		t.nodes[e.ID] = n
		t.order = append(t.order, e.ID)
	}
	if e.ParentID != "" {
		n.parents.Add(e.ParentID)
		// Layers shared by several images receive the same events once per
		// image. Accept updates from the first declared parent only, so the
		// rendered state doesn't flicker between concurrent pull streams.
		if n.anchor != e.ParentID {
			return
		}
	}

	wasCompleted := n.completed()
	n.status = e.Status
	n.text = e.Text
	n.details = e.Details
	// progress is monotonic: out-of-order events must not move bars backwards
	n.total = max(n.total, e.Total)
	n.current = max(n.current, e.Current)
	n.percent = max(n.percent, e.Percent)
	if n.completed() && !wasCompleted {
		n.endedAt = now
	}
}

// roots returns the top-level tasks in arrival order.
func (t *taskTree) roots() []*node {
	var roots []*node
	for _, id := range t.order {
		if n := t.nodes[id]; len(n.parents) == 0 {
			roots = append(roots, n)
		}
	}
	return roots
}

// children returns the tasks attached to parent, in arrival order. A layer
// shared by several images is listed under each of them.
func (t *taskTree) children(parent string) []*node {
	var children []*node
	for _, id := range t.order {
		if n := t.nodes[id]; n.parents.Has(parent) {
			children = append(children, n)
		}
	}
	return children
}

// counts returns completed and total task counts, children included, to
// preserve the historical "[+] pull 3/15" header semantics.
func (t *taskTree) counts() (done, total int) {
	for _, n := range t.nodes {
		if n.completed() {
			done++
		}
	}
	return done, len(t.nodes)
}
