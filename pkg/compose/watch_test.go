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
	"testing"

	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/assert"
)

func Test_debounce(t *testing.T) {
	ch := make(chan string)
	var (
		ran int
		got []string
	)
	clock := clockwork.NewFakeClock()
	ctx, stop := context.WithCancel(context.TODO())
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		debounce(ctx, clock, quietPeriod, ch, func(services []string) {
			got = append(got, services...)
			ran++
			stop()
		})
		return nil
	})
	for i := 0; i < 100; i++ {
		ch <- "test"
	}
	assert.Equal(t, ran, 0)
	clock.Advance(quietPeriod)
	err := eg.Wait()
	assert.NilError(t, err)
	assert.Equal(t, ran, 1)
	assert.DeepEqual(t, got, []string{"test"})
}
