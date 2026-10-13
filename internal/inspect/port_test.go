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

package inspect

import (
	"testing"

	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"
)

// TestContainerPublishersPreservesTieOrder guards against containerPublishers
// reordering same-PrivatePort entries (e.g. a dual-stack publish exposing the
// same target port on both an IPv4 and an IPv6 host address): Ports() picks
// publishers[0] as *the* answer for a single-port lookup, so a sort that
// merely orders by PrivatePort must not additionally scramble ties.
//
// sort.Slice's pdqsort falls back to a (stable) insertion sort for slices of
// <=12 elements, so a small fixture can't tell a stable sort from an unstable
// one; this uses enough tied groups to force the quicksort partitioning path.
func TestContainerPublishersPreservesTieOrder(t *testing.T) {
	var ports []container.PortSummary
	privatePorts := []int{80, 53, 443}
	for _, pp := range privatePorts {
		for i := range 5 {
			ports = append(ports, container.PortSummary{
				PrivatePort: uint16(pp),
				PublicPort:  uint16(i), // tags each tied entry with its input position
				Type:        "tcp",
			})
		}
	}
	ports = append(ports, container.PortSummary{PrivatePort: 22, PublicPort: 0, Type: "tcp"})

	ctr := container.Summary{ID: "123", Ports: ports}

	got := containerPublishers(ctr)

	lastPosByTargetPort := map[int]int{}
	for _, p := range got {
		pos := p.PublishedPort
		if prev, ok := lastPosByTargetPort[p.TargetPort]; ok {
			assert.Assert(t, pos > prev, "tied entries for private port %d out of input order: got position %d after %d", p.TargetPort, pos, prev)
		}
		lastPosByTargetPort[p.TargetPort] = pos
	}
}
