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

// Package concurrency holds the helpers Compose uses to bound how many
// operations run at once, driven by the --parallel setting.
//
// Across Compose, a bound of zero or less means "unlimited" (the Go zero value
// of a configured maximum must never stall a command). This is not what the
// underlying primitives do: errgroup.SetLimit(0) allows zero goroutines, so a
// plain SetLimit call with an unset maximum hangs forever. Every helper here
// applies the "<= 0 means unlimited" rule in one place.
//
// The package is a leaf: it depends on no other Compose package.
package concurrency

import (
	"context"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// NewErrgroup returns an errgroup.Group, with the context it derives, running
// at most maxConcurrency goroutines at once. maxConcurrency <= 0 leaves it
// unlimited, since errgroup.SetLimit(0) means "allow zero goroutines", not
// "unlimited".
func NewErrgroup(ctx context.Context, maxConcurrency int) (*errgroup.Group, context.Context) {
	eg, ctx := errgroup.WithContext(ctx)
	if maxConcurrency > 0 {
		eg.SetLimit(maxConcurrency)
	}
	return eg, ctx
}

// NewLimiter returns a semaphore bounding concurrency to maxConcurrency, or
// nil when maxConcurrency <= 0 (unlimited). Use it, with Acquire and Release,
// to gate only part of a goroutine's work (e.g. the opening call of an
// indefinite stream, not the stream itself), or to share one budget between
// several concurrent calls (see ForEach), where NewErrgroup's whole-goroutine
// bound does not apply.
func NewLimiter(maxConcurrency int) *semaphore.Weighted {
	if maxConcurrency <= 0 {
		return nil
	}
	return semaphore.NewWeighted(int64(maxConcurrency))
}

// Acquire takes a slot from limiter, or is a no-op when limiter is nil.
func Acquire(ctx context.Context, limiter *semaphore.Weighted) error {
	if limiter == nil {
		return nil
	}
	return limiter.Acquire(ctx, 1)
}

// Release gives back a slot taken via Acquire, or is a no-op when limiter is
// nil.
func Release(limiter *semaphore.Weighted) {
	if limiter != nil {
		limiter.Release(1)
	}
}

// PanicSafeRelease is deferred by callers that release a slot earlier than
// function exit on the success path, to avoid leaking it if the guarded call
// panics before reaching that point. It is a no-op unless the deferring
// goroutine is unwinding from a panic, in which case it releases the slot and
// re-panics. It must be deferred directly (recover only works there).
func PanicSafeRelease(limiter *semaphore.Weighted) {
	if p := recover(); p != nil {
		Release(limiter)
		panic(p)
	}
}

// ForEach runs fn concurrently for each item, bounded by limiter, and returns
// the first error, cancelling the context passed to the other calls.
// Unlike NewErrgroup, limiter is built by the caller and can be shared across
// several concurrently-dispatched calls, so the combined concurrency of all of
// them never exceeds the configured budget. A nil limiter means unbounded.
func ForEach[T any](ctx context.Context, limiter *semaphore.Weighted, items []T, fn func(context.Context, T) error) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, item := range items {
		eg.Go(func() error {
			if err := Acquire(ctx, limiter); err != nil {
				return err
			}
			defer Release(limiter)
			return fn(ctx, item)
		})
	}
	return eg.Wait()
}

// ForEachBounded runs fn concurrently for each item, at most maxConcurrency at a
// time (0 meaning no bound), and returns the first error, cancelling the context
// passed to the other calls. Use ForEach instead when the concurrency budget must
// be shared across several concurrently-dispatched calls, e.g. one per service
// visited by InDependencyOrder.
func ForEachBounded[T any](ctx context.Context, maxConcurrency int, items []T, fn func(context.Context, T) error) error {
	eg, ctx := NewErrgroup(ctx, maxConcurrency)
	for _, item := range items {
		eg.Go(func() error {
			return fn(ctx, item)
		})
	}
	return eg.Wait()
}
