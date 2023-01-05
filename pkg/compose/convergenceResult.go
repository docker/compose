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

package compose

import (
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/docker/api/types"
)

// ConvergenceResult tracks operations applied to containers during a convergence execution
type ConvergenceResult struct {
	containers Containers
	operations map[string]int
}

const (
	containerCreated = iota
	containerRecreated
	containerStarted
	containerRemoved
)

func newConvergenceResult() *ConvergenceResult {
	return &ConvergenceResult{
		operations: map[string]int{},
	}
}

func (r *ConvergenceResult) add(c types.Container, op int) {
	r.containers = append(r.containers, c)
	r.operations[c.ID] = op
}

func (r *ConvergenceResult) addAll(o *ConvergenceResult) {
	for _, c := range o.containers {
		r.containers = append(r.containers, c)
		r.operations[c.ID] = o.operations[c.ID]
	}
}

// upServices collects services which have been started or created (i.e switched from "down" to "up" state)
// this excludes services which where already running, even if those have been updated/scaled
func (r *ConvergenceResult) upServices() []string {
	created := utils.Set[string]{}
	for _, container := range r.containers {
		op := r.operations[container.ID]
		switch op {
		case containerCreated, containerStarted:
			created.Add(container.Labels[api.ServiceLabel])
		case containerRecreated:
			created.Remove(container.Labels[api.ServiceLabel])
		}
	}
	return created.Values()
}
