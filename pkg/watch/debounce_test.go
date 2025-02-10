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

package watch

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"gotest.tools/v3/assert"
)

func Test_BatchDebounceEvents(t *testing.T) {
	ch := make(chan FileEvent)
	clock := clockwork.NewFakeClock()
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)

	eventBatchCh := BatchDebounceEvents(ctx, clock, ch)
	for i := 0; i < 100; i++ {
		path := "/a"
		if i%2 == 0 {
			path = "/b"
		}

		ch <- FileEvent(path)
	}
	// we sent 100 events + the debouncer
	err := clock.BlockUntilContext(ctx, 101)
	assert.NilError(t, err)
	clock.Advance(QuietPeriod)
	select {
	case batch := <-eventBatchCh:
		slices.Sort(batch)
		assert.Equal(t, len(batch), 2)
		assert.Equal(t, batch[0], FileEvent("/a"))
		assert.Equal(t, batch[1], FileEvent("/b"))
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timed out waiting for events")
	}
	err = clock.BlockUntilContext(ctx, 1)
	assert.NilError(t, err)
	clock.Advance(QuietPeriod)

	// there should only be a single batch
	select {
	case batch := <-eventBatchCh:
		t.Fatalf("unexpected events: %v", batch)
	case <-time.After(50 * time.Millisecond):
		// channel is empty
	}
}
