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
	"errors"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/jsonstream"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestDecodeProgressStream(t *testing.T) {
	var ids []string
	err := decodeProgressStream(strings.NewReader(`{"id":"a"}{"id":"b"}`), func(jm jsonstream.Message) { ids = append(ids, jm.ID) })
	assert.NilError(t, err)
	assert.DeepEqual(t, ids, []string{"a", "b"})

	ids = nil
	err = decodeProgressStream(strings.NewReader(`{"id":"a"}{"errorDetail":{"message":"boom"}}{"id":"c"}`), func(jm jsonstream.Message) { ids = append(ids, jm.ID) })
	assert.Error(t, err, "boom")
	assert.DeepEqual(t, ids, []string{"a"})

	assert.Assert(t, decodeProgressStream(strings.NewReader(`{"id":`), func(jsonstream.Message) {}) != nil)
}

func TestProgressPercent(t *testing.T) {
	assert.Equal(t, progressPercent(50, 200), 25)
	assert.Equal(t, progressPercent(300, 200), 100)
}

// doneRecorder records the success flag passed to Done.
type doneRecorder struct {
	api.EventProcessor
	calls []bool
}

func (r *doneRecorder) Start(context.Context, string) {}
func (r *doneRecorder) Done(_ string, success bool)   { r.calls = append(r.calls, success) }

func TestRunReportsSuccessToEventProcessor(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"operation succeeded": {err: nil, want: true},
		"operation failed":    {err: errors.New("boom"), want: false},
	} {
		t.Run(name, func(t *testing.T) {
			bus := &doneRecorder{}
			err := Run(t.Context(), func(context.Context) error { return tc.err }, "test", bus)
			assert.Equal(t, err, tc.err)
			assert.DeepEqual(t, bus.calls, []bool{tc.want})
		})
	}
}
