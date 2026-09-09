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
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// fakeRebuilder is a deterministic, channel-driven stand-in for the real
// `s.rebuild` call. Every invocation:
//   - records the requested services (sorted, for easy comparison),
//   - blocks until the test sends a value (nil or an error) on `release`,
//   - fails the test via t.Errorf (safe to call from any goroutine) if it
//     is ever entered while another invocation hasn't returned yet, which
//     is exactly the "no concurrent rebuild" property under test.
type fakeRebuilder struct {
	t *testing.T

	running int32 // atomic: 1 while an invocation is in flight

	started chan []string // signaled with sorted services each time rebuild() is entered
	release chan error    // test sends here to let the current invocation return

	mu    sync.Mutex
	calls [][]string // sorted services for every completed invocation, in order
}

func newFakeRebuilder(t *testing.T) *fakeRebuilder {
	t.Helper()
	return &fakeRebuilder{
		t:       t,
		started: make(chan []string),
		release: make(chan error),
	}
}

func (f *fakeRebuilder) rebuild(services []string) error {
	if !atomic.CompareAndSwapInt32(&f.running, 0, 1) {
		f.t.Errorf("rebuild() invoked while a previous rebuild was still in progress: services=%v", services)
	}
	defer atomic.StoreInt32(&f.running, 0)

	sorted := append([]string(nil), services...)
	sort.Strings(sorted)

	f.mu.Lock()
	f.calls = append(f.calls, sorted)
	f.mu.Unlock()

	f.started <- sorted
	return <-f.release
}

func (f *fakeRebuilder) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// awaitStarted waits (with a bounded, generous timeout so a genuine bug
// hangs the test instead of the suite) for the next rebuild invocation to
// begin, returning the sorted services it was called with.
func awaitStarted(t *testing.T, f *fakeRebuilder) []string {
	t.Helper()
	select {
	case services := <-f.started:
		return services
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuild to start")
		return nil
	}
}

// assertNoRebuildStarts asserts that no new rebuild invocation begins
// within a short grace window. Since a correctly implemented scheduler
// decides synchronously (under its own lock) whether to start a rebuild or
// merely record it as pending, this is not a race against a slow producer:
// if the scheduler doesn't start a rebuild by the time this is called, it
// never will for the requests already issued.
func assertNoRebuildStarts(t *testing.T, f *fakeRebuilder) {
	t.Helper()
	select {
	case services := <-f.started:
		t.Fatalf("unexpected rebuild started with services=%v", services)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing started
	}
}

// TestRebuildScheduler_FirstRequestStartsImmediately covers behavior (1):
// a request issued while idle must trigger a rebuild without being delayed
// or batched -- latency for an isolated edit must be unchanged.
func TestRebuildScheduler_FirstRequestStartsImmediately(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	scheduler.Request([]string{"web"})

	services := awaitStarted(t, f)
	assert.DeepEqual(t, services, []string{"web"})

	f.release <- nil
	scheduler.Wait()

	assert.Equal(t, f.callCount(), 1)
}

// TestRebuildScheduler_RequestsDuringBuildAreCoalescedIntoPending covers
// behavior (2): requests arriving while a rebuild is already running must
// not start a new rebuild; they accumulate into a deduplicated pending set.
func TestRebuildScheduler_RequestsDuringBuildAreCoalescedIntoPending(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	scheduler.Request([]string{"web"})
	first := awaitStarted(t, f)
	assert.DeepEqual(t, first, []string{"web"})

	// These arrive while the first rebuild is still in flight (we haven't
	// released it yet) and must not trigger immediate rebuilds.
	scheduler.Request([]string{"api"})
	scheduler.Request([]string{"web"}) // duplicate, must not double up
	scheduler.Request([]string{"api", "worker"})

	assertNoRebuildStarts(t, f)
	assert.Equal(t, f.callCount(), 1)

	f.release <- nil

	// The pending requests above must still produce exactly one trailing
	// rebuild (verified by TestRebuildScheduler_TrailingRebuildConsolidatesPendingServices);
	// drain it here so Wait() isn't left blocking on it forever.
	awaitStarted(t, f)
	f.release <- nil
	scheduler.Wait()
}

// TestRebuildScheduler_TrailingRebuildConsolidatesPendingServices covers
// behavior (3): once the in-progress rebuild completes, exactly one
// trailing rebuild starts automatically, covering the union of every
// service requested while the first rebuild was running, and the pending
// set is cleared.
func TestRebuildScheduler_TrailingRebuildConsolidatesPendingServices(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	scheduler.Request([]string{"web"})
	awaitStarted(t, f)

	scheduler.Request([]string{"api"})
	scheduler.Request([]string{"web"})
	scheduler.Request([]string{"api", "worker"})

	f.release <- nil // let the first rebuild finish

	trailing := awaitStarted(t, f)
	assert.DeepEqual(t, trailing, []string{"api", "web", "worker"})

	f.release <- nil
	scheduler.Wait()

	// Exactly one trailing rebuild: nothing else was requested during it.
	assert.Equal(t, f.callCount(), 2)
}

// TestRebuildScheduler_TrailingRebuildsChainUntilPendingEmpty covers
// behavior (4): if new requests arrive while a trailing rebuild is
// running, another trailing rebuild fires when it completes, and this
// repeats until a rebuild finishes with nothing pending, at which point the
// scheduler goes idle.
func TestRebuildScheduler_TrailingRebuildsChainUntilPendingEmpty(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	scheduler.Request([]string{"web"})
	assert.DeepEqual(t, awaitStarted(t, f), []string{"web"})

	scheduler.Request([]string{"api"})
	f.release <- nil // first rebuild done, "api" is pending -> triggers 2nd rebuild

	assert.DeepEqual(t, awaitStarted(t, f), []string{"api"})

	// A request arrives *during* the trailing rebuild: must chain into a 3rd.
	scheduler.Request([]string{"db"})
	f.release <- nil // 2nd rebuild done, "db" is pending -> triggers 3rd rebuild

	assert.DeepEqual(t, awaitStarted(t, f), []string{"db"})

	// Nothing requested during the 3rd rebuild: scheduler must return to idle.
	f.release <- nil
	scheduler.Wait()

	assertNoRebuildStarts(t, f)
	assert.Equal(t, f.callCount(), 3)
}

// TestRebuildScheduler_RebuildErrorDoesNotBlockPendingProcessing covers
// behavior (6): a failed rebuild must not prevent a subsequently pending
// rebuild from being processed.
func TestRebuildScheduler_RebuildErrorDoesNotBlockPendingProcessing(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	scheduler.Request([]string{"web"})
	awaitStarted(t, f)

	scheduler.Request([]string{"api"})
	f.release <- errors.New("build failed") // first rebuild fails

	// Despite the error, the pending "api" request must still be processed.
	trailing := awaitStarted(t, f)
	assert.DeepEqual(t, trailing, []string{"api"})

	f.release <- nil
	scheduler.Wait()

	assert.Equal(t, f.callCount(), 2)
}

// TestRebuildScheduler_NoConcurrentRebuilds covers behavior (5): at most
// one rebuild must ever be running at a time, and the scheduler's internal
// state (pending set, "building" flag) must not race under concurrent
// requests. Run with `go test -race` to make both properties meaningful.
func TestRebuildScheduler_NoConcurrentRebuilds(t *testing.T) {
	var running int32
	var maxObservedConcurrency int32

	rebuild := func(_ []string) error {
		n := atomic.AddInt32(&running, 1)
		defer atomic.AddInt32(&running, -1)

		for {
			max := atomic.LoadInt32(&maxObservedConcurrency)
			if n <= max || atomic.CompareAndSwapInt32(&maxObservedConcurrency, max, n) {
				break
			}
		}

		// Widen the window during which a concurrency bug would be
		// observable. This does not make the test's pass/fail outcome
		// depend on timing: it only increases the odds of *catching* a
		// bug, it can never cause a false failure.
		time.Sleep(2 * time.Millisecond)
		return nil
	}

	scheduler := newRebuildScheduler(rebuild)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			scheduler.Request([]string{"svc"})
		}()
	}
	wg.Wait()

	scheduler.Wait()

	assert.Equal(t, atomic.LoadInt32(&maxObservedConcurrency), int32(1))
}

// TestRebuildScheduler_InFlightOrPending covers the query callers use to
// avoid racing a plain restart against a rebuild for the same service,
// whether that rebuild is currently running or only queued as trailing.
func TestRebuildScheduler_InFlightOrPending(t *testing.T) {
	f := newFakeRebuilder(t)
	scheduler := newRebuildScheduler(f.rebuild)

	assert.Equal(t, scheduler.InFlightOrPending("web"), false)

	scheduler.Request([]string{"web"})
	awaitStarted(t, f)
	assert.Equal(t, scheduler.InFlightOrPending("web"), true)
	assert.Equal(t, scheduler.InFlightOrPending("api"), false)

	// Queued for the trailing rebuild while "web" is still building.
	scheduler.Request([]string{"api"})
	assert.Equal(t, scheduler.InFlightOrPending("api"), true)

	f.release <- nil // "web" finishes, "api" starts as the trailing rebuild
	awaitStarted(t, f)
	assert.Equal(t, scheduler.InFlightOrPending("api"), true)
	assert.Equal(t, scheduler.InFlightOrPending("web"), false)

	f.release <- nil
	scheduler.Wait()

	assert.Equal(t, scheduler.InFlightOrPending("api"), false)
}
