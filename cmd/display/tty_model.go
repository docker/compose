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
//
// Roots, per-parent children and the completed count are maintained
// incrementally by apply: the layout reads them on every refresh tick, so a
// frame with an unchanged tree costs O(1) here instead of a full rescan.
//
// Every time value handed to apply (and to the layout) must come from the
// writer's single clock: the model relies on time never regressing between
// calls — spinner frames and elapsed timers are computed as bare
// differences against startedAt/endedAt.
type taskTree struct {
	nodes    map[string]*node
	roots    []*node            // nodes without any parent, in arrival order
	children map[string][]*node // parent id → children, in link-arrival order
	done     int                // nodes currently in a completed status
}

func newTaskTree() taskTree {
	return taskTree{
		nodes:    map[string]*node{},
		children: map[string][]*node{},
	}
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
		if e.ParentID == "" {
			t.roots = append(t.roots, n)
		}
	}
	if e.ParentID != "" {
		if !n.parents.Has(e.ParentID) {
			n.parents.Add(e.ParentID)
			t.children[e.ParentID] = append(t.children[e.ParentID], n)
			if len(n.parents) == 1 {
				// first parent ever: the node is not a root after all
				t.removeRoot(n)
			}
		}
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
	switch {
	case n.completed() && !wasCompleted:
		n.endedAt = now
		t.done++
	case !n.completed() && wasCompleted:
		t.done--
	}
}

func (t *taskTree) removeRoot(n *node) {
	for i, root := range t.roots {
		if root == n {
			t.roots = append(t.roots[:i], t.roots[i+1:]...)
			return
		}
	}
}

// counts returns completed and total task counts, children included, to
// preserve the historical "[+] pull 3/15" header semantics.
func (t *taskTree) counts() (done, total int) {
	return t.done, len(t.nodes)
}
