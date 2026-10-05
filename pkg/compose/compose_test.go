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

package compose

import (
	"fmt"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// TestNewLimitedErrgroup_NonPositiveIsUnlimited guards against the bug this
// helper exists to fix: errgroup.SetLimit(0) means "allow zero goroutines",
// not "unlimited". A composeService{} built without going through
// NewComposeService has maxConcurrency's Go zero-value (0), so an
// unconditional SetLimit(maxConcurrency) at any call site would silently
// hang forever instead of running.
func TestNewLimitedErrgroup_NonPositiveIsUnlimited(t *testing.T) {
	for _, maxConcurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxConcurrency=%d", maxConcurrency), func(t *testing.T) {
			eg, _ := newLimitedErrgroup(t.Context(), maxConcurrency)

			done := make(chan struct{})
			go func() {
				for range 5 {
					eg.Go(func() error { return nil })
				}
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("eg.Go blocked: maxConcurrency <= 0 must mean unlimited, not SetLimit(0) (zero goroutines allowed)")
			}
			assert.NilError(t, eg.Wait())
		})
	}
}
