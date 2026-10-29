/*
   Copyright 2026 Docker Compose CLI authors

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

package concurrency

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"gotest.tools/v3/assert"
)

// peakTracker records the highest number of calls inside it at once.
type peakTracker struct {
	cur, peak atomic.Int64
}

func (p *peakTracker) enter() {
	n := p.cur.Add(1)
	for {
		old := p.peak.Load()
		if n <= old || p.peak.CompareAndSwap(old, n) {
			return
		}
	}
}

func (p *peakTracker) exit() { p.cur.Add(-1) }

// TestNewErrgroup_NonPositiveIsUnlimited guards against the bug this package
// exists to prevent: errgroup.SetLimit(0) means "allow zero goroutines", not
// "unlimited". The Go zero value of a maximum (a service built without going
// through the usual constructor) must not stall.
func TestNewErrgroup_NonPositiveIsUnlimited(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			const goroutines = 8
			eg, _ := NewErrgroup(t.Context(), limit)

			// every goroutine waits for all the others to be running: this can
			// only complete if none of them is held back by a limit.
			var started sync.WaitGroup
			started.Add(goroutines)
			release := make(chan struct{})
			for range goroutines {
				eg.Go(func() error {
					started.Done()
					<-release
					return nil
				})
			}
			started.Wait()
			close(release)
			assert.NilError(t, eg.Wait())
		})
	}
}

func TestNewErrgroup_BoundsConcurrency(t *testing.T) {
	const (
		limit = 3
		tasks = 20
	)
	eg, _ := NewErrgroup(t.Context(), limit)

	var tracker peakTracker
	// the first limit tasks hold their slot until all of them are in, proving
	// the bound is reached; the others then flow through.
	var gate sync.WaitGroup
	gate.Add(limit)
	var first atomic.Int64
	for range tasks {
		eg.Go(func() error {
			tracker.enter()
			defer tracker.exit()
			if first.Add(1) <= limit {
				gate.Done()
				gate.Wait()
			}
			return nil
		})
	}
	assert.NilError(t, eg.Wait())
	assert.Equal(t, tracker.peak.Load(), int64(limit))
}

func TestNewErrgroup_FirstErrorCancelsContext(t *testing.T) {
	eg, ctx := NewErrgroup(t.Context(), 0)
	boom := errors.New("boom")

	eg.Go(func() error { return boom })
	eg.Go(func() error {
		<-ctx.Done() // only returns once the first error cancelled the context
		return ctx.Err()
	})

	assert.Assert(t, errors.Is(eg.Wait(), boom))
	assert.Assert(t, errors.Is(context.Cause(ctx), boom))
}

func TestNewLimiter(t *testing.T) {
	assert.Assert(t, NewLimiter(0) == nil)
	assert.Assert(t, NewLimiter(-1) == nil)

	limiter := NewLimiter(2)
	assert.Assert(t, limiter != nil)
	assert.Assert(t, limiter.TryAcquire(1))
	assert.Assert(t, limiter.TryAcquire(1))
	assert.Assert(t, !limiter.TryAcquire(1), "a third slot must not be available")
}

func TestAcquireRelease_NilLimiterIsNoop(t *testing.T) {
	assert.NilError(t, Acquire(t.Context(), nil))
	Release(nil)
}

func TestAcquire_HonorsContext(t *testing.T) {
	limiter := NewLimiter(1)
	assert.NilError(t, Acquire(t.Context(), limiter))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.Assert(t, errors.Is(Acquire(ctx, limiter), context.Canceled))

	Release(limiter)
	assert.NilError(t, Acquire(t.Context(), limiter))
}

func TestPanicSafeRelease(t *testing.T) {
	t.Run("releases and re-panics", func(t *testing.T) {
		limiter := NewLimiter(1)
		assert.NilError(t, Acquire(t.Context(), limiter))

		func() {
			defer func() {
				assert.Equal(t, recover(), "kaboom")
			}()
			defer PanicSafeRelease(limiter)
			panic("kaboom")
		}()

		assert.Assert(t, limiter.TryAcquire(1), "the slot must have been released")
	})

	t.Run("does not release without panic", func(t *testing.T) {
		limiter := NewLimiter(1)
		assert.NilError(t, Acquire(t.Context(), limiter))

		func() {
			defer PanicSafeRelease(limiter)
			Release(limiter) // the success path releases early on its own
		}()

		// a double release would panic on the next one: exactly one slot is free
		assert.Assert(t, limiter.TryAcquire(1))
		assert.Assert(t, !limiter.TryAcquire(1))
	})

	t.Run("nil limiter", func(t *testing.T) {
		func() {
			defer func() { assert.Equal(t, recover(), "kaboom") }()
			defer PanicSafeRelease(nil)
			panic("kaboom")
		}()
	})
}

func TestForEach_NilLimiterIsUnbounded(t *testing.T) {
	const items = 8
	var started sync.WaitGroup
	started.Add(items)
	release := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		done <- ForEach(t.Context(), nil, make([]int, items), func(context.Context, int) error {
			started.Done()
			<-release
			return nil
		})
	}()
	started.Wait() // all items run at once
	close(release)
	assert.NilError(t, <-done)
}

func TestForEach_BoundsConcurrency(t *testing.T) {
	const (
		limit = 3
		items = 20
	)
	limiter := NewLimiter(limit)

	var tracker peakTracker
	var gate sync.WaitGroup
	gate.Add(limit)
	var first atomic.Int64
	err := ForEach(t.Context(), limiter, make([]int, items), func(context.Context, int) error {
		tracker.enter()
		defer tracker.exit()
		if first.Add(1) <= limit {
			gate.Done()
			gate.Wait() // the bound is reached before anyone leaves
		}
		return nil
	})
	assert.NilError(t, err)
	assert.Equal(t, tracker.peak.Load(), int64(limit))
}

func TestForEach_VisitsEveryItem(t *testing.T) {
	var sum atomic.Int64
	err := ForEach(t.Context(), NewLimiter(2), []int{1, 2, 3, 4, 5}, func(_ context.Context, n int) error {
		sum.Add(int64(n))
		return nil
	})
	assert.NilError(t, err)
	assert.Equal(t, sum.Load(), int64(15))
}

func TestForEach_FirstErrorCancelsTheOthers(t *testing.T) {
	boom := errors.New("boom")
	started := make(chan struct{})

	err := ForEach(t.Context(), NewLimiter(2), []int{0, 1}, func(ctx context.Context, n int) error {
		if n == 0 {
			<-started // the sibling is running before we fail
			return boom
		}
		close(started)
		<-ctx.Done() // only returns once the failure cancelled the context
		return ctx.Err()
	})
	assert.Assert(t, errors.Is(err, boom))
}

// TestForEach_SharedLimiterBoundsCombinedConcurrency: two concurrent calls
// drawing on one limiter must never run more than the budget between them.
func TestForEach_SharedLimiterBoundsCombinedConcurrency(t *testing.T) {
	const (
		limit = 3
		items = 10
	)
	limiter := NewLimiter(limit)

	var tracker peakTracker
	var gate sync.WaitGroup
	gate.Add(limit)
	var first atomic.Int64
	fn := func(context.Context, int) error {
		tracker.enter()
		defer tracker.exit()
		if first.Add(1) <= limit {
			gate.Done()
			gate.Wait()
		}
		return nil
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = ForEach(t.Context(), limiter, make([]int, items), fn)
		}()
	}
	wg.Wait()

	assert.NilError(t, errors.Join(errs...))
	assert.Equal(t, tracker.peak.Load(), int64(limit))
}

func TestForEach_AcquireFailureWhenContextCancelled(t *testing.T) {
	limiter := NewLimiter(1)
	assert.NilError(t, Acquire(t.Context(), limiter)) // budget exhausted

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	err := ForEach(ctx, limiter, []int{1}, func(context.Context, int) error {
		called = true
		return nil
	})
	assert.Assert(t, errors.Is(err, context.Canceled))
	assert.Assert(t, !called)
}

func TestForEachBounded_VisitsEveryItem(t *testing.T) {
	var sum atomic.Int64
	err := ForEachBounded(t.Context(), 2, []int{1, 2, 3, 4, 5}, func(_ context.Context, n int) error {
		sum.Add(int64(n))
		return nil
	})
	assert.NilError(t, err)
	assert.Equal(t, sum.Load(), int64(15))
}
