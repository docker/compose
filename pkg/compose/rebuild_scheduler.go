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
	"context"
	"sync"

	"github.com/docker/compose/v5/pkg/utils"
)

// rebuildFunc performs a rebuild for the given services. The context is the
// run's own: the scheduler cancels it when the run is interrupted because a
// newer request made its outcome stale, and when the watch shuts down.
type rebuildFunc func(ctx context.Context, services []string) error

// rebuildScheduler coalesces rebuild requests received while a rebuild is
// already running into a single trailing rebuild, so that a burst of file
// change events never queues up more than one extra rebuild.
//
// Its convergence invariant: every request ends up covered by a rebuild
// whose build-context snapshot was taken after the request. A run made
// stale by a newer request for one of its own services is interrupted
// rather than left to finish a result that would be replaced anyway.
type rebuildScheduler struct {
	ctx     context.Context // watch lifecycle: no run outlives it
	rebuild rebuildFunc

	mu        sync.Mutex
	building  bool
	active    utils.Set[string]  // services being rebuilt by the current run, if any
	pending   utils.Set[string]  // services queued for the next trailing rebuild
	interrupt context.CancelFunc // cancels the current run; nil outside a run
	wg        sync.WaitGroup
}

func newRebuildScheduler(ctx context.Context, rebuild rebuildFunc) *rebuildScheduler {
	return &rebuildScheduler{
		ctx:     ctx,
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
//
// A request naming a service the current run is already rebuilding makes
// that run stale — its build context was snapshotted before the change
// behind this request — so the run is interrupted. Interruption kills the
// whole run, so its complete active set is folded back into pending: the
// trailing rebuild redoes every affected service against a fresh snapshot.
func (s *rebuildScheduler) Request(services []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.AddAll(services...)
	if !s.building {
		s.building = true
		s.wg.Add(1)
		go s.run()
		return
	}
	for _, service := range services {
		if s.active.Has(service) {
			s.pending.AddAll(s.active.Elements()...)
			s.interrupt()
			return
		}
	}
}

// Pending reports whether a rebuild for the given service is queued for the
// next trailing rebuild. Such a rebuild will snapshot the build context
// after whatever change the caller is reacting to: recreating and starting
// the service from it makes a separate restart redundant.
func (s *rebuildScheduler) Pending(service string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending.Has(service)
}

// InFlight reports whether the given service is being rebuilt by the
// current run. That run's snapshot predates the caller's change: acting on
// the service's container would race the run's create/start, and the run's
// outcome is already stale — fold the action into a Request instead.
func (s *rebuildScheduler) InFlight(service string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active.Has(service)
}

// run drains the pending set into a rebuild, looping to pick up whatever
// accumulated (or was folded back by an interruption) while that rebuild
// was in flight, until nothing is left or the watch shuts down.
func (s *rebuildScheduler) run() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		if len(s.pending) == 0 || s.ctx.Err() != nil {
			s.building = false
			s.mu.Unlock()
			return
		}
		runCtx, cancel := context.WithCancel(s.ctx)
		s.interrupt = cancel
		s.active = s.pending
		s.pending = utils.NewSet[string]()
		services := s.active.Elements()
		s.mu.Unlock()

		// The error is deliberately not handled here: rebuild reports every
		// failure to the user log itself, and an interrupted run's services
		// are already folded back into pending by Request.
		_ = s.rebuild(runCtx, services)

		s.mu.Lock()
		s.active = nil
		s.interrupt = nil
		s.mu.Unlock()
		cancel()
	}
}

// Wait blocks until the scheduler is idle: no rebuild is running and nothing
// is pending. The caller must have stopped issuing Requests.
func (s *rebuildScheduler) Wait() {
	s.wg.Wait()
}
