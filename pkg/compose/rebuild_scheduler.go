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
	"sync"

	"github.com/docker/compose/v5/pkg/utils"
)

// rebuildFunc performs a rebuild for the given services.
type rebuildFunc func(services []string) error

// rebuildScheduler coalesces rebuild requests received while a rebuild is
// already running into a single trailing rebuild, so that a burst of file
// change events never queues up more than one extra rebuild.
type rebuildScheduler struct {
	rebuild rebuildFunc

	mu       sync.Mutex
	building bool
	active   utils.Set[string] // services being rebuilt by the current run, if any
	pending  utils.Set[string] // services queued for the next trailing rebuild
	wg       sync.WaitGroup
}

func newRebuildScheduler(rebuild rebuildFunc) *rebuildScheduler {
	return &rebuildScheduler{
		rebuild: rebuild,
		pending: utils.NewSet[string](),
	}
}

// Request asks for a rebuild of services. It never blocks on the rebuild
// itself: the services are always merged into the pending set, and the run
// loop is (re)started only if it isn't already draining it.
//
// The decision must be made synchronously, under the lock, so that requests
// racing with the run loop's completion are never lost nor cause two
// rebuilds to run concurrently.
func (s *rebuildScheduler) Request(services []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.AddAll(services...)
	if s.building {
		return
	}
	s.building = true
	s.wg.Add(1)
	go s.run()
}

// InFlightOrPending reports whether a rebuild for the given service is
// currently running or queued for the next trailing rebuild. Callers use
// this to avoid racing a plain restart against a rebuild started by an
// earlier, still-running batch.
func (s *rebuildScheduler) InFlightOrPending(service string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active.Has(service) || s.pending.Has(service)
}

// run drains the pending set into a rebuild, looping to pick up whatever
// accumulated while that rebuild was in flight, until nothing is left.
func (s *rebuildScheduler) run() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		s.active = s.pending
		s.pending = utils.NewSet[string]()
		services := s.active.Elements()
		s.mu.Unlock()

		_ = s.rebuild(services)

		s.mu.Lock()
		s.active = nil
		if len(s.pending) == 0 {
			s.building = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
	}
}

// Wait blocks until the scheduler is idle: no rebuild is running and nothing
// is pending.
func (s *rebuildScheduler) Wait() {
	s.wg.Wait()
}
