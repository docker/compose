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
	"time"

	"github.com/docker/compose/v2/pkg/utils"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
)

const QuietPeriod = 500 * time.Millisecond

// batchDebounceEvents groups identical file events within a sliding time window and writes the results to the returned
// channel.
//
// The returned channel is closed when the debouncer is stopped via context cancellation or by closing the input channel.
func BatchDebounceEvents(ctx context.Context, clock clockwork.Clock, input <-chan FileEvent) <-chan []FileEvent {
	out := make(chan []FileEvent)
	go func() {
		defer close(out)
		seen := utils.Set[FileEvent]{}
		flushEvents := func() {
			if len(seen) == 0 {
				return
			}
			logrus.Debugf("flush: %d events %s", len(seen), seen)

			events := make([]FileEvent, 0, len(seen))
			for e := range seen {
				events = append(events, e)
			}
			out <- events
			seen = utils.Set[FileEvent]{}
		}

		t := clock.NewTicker(QuietPeriod)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.Chan():
				flushEvents()
			case e, ok := <-input:
				if !ok {
					// input channel was closed
					flushEvents()
					return
				}
				if _, ok := seen[e]; !ok {
					seen.Add(e)
				}
				t.Reset(QuietPeriod)
			}
		}
	}()
	return out
}
