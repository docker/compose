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

// doneRecorder is an api.EventProcessor keeping the arguments received by Done.
type doneRecorder struct {
	operation string
	success   bool
	calls     int
}

func (r *doneRecorder) Start(context.Context, string) {}

func (r *doneRecorder) On(...api.Resource) {}

func (r *doneRecorder) Done(operation string, success bool) {
	r.operation = operation
	r.success = success
	r.calls++
}

func TestRunReportsSuccessToDone(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		err     error
		success bool
	}{
		{name: "operation succeeds", err: nil, success: true},
		{name: "operation fails", err: boom, success: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &doneRecorder{}
			err := Run(t.Context(), func(context.Context) error { return tt.err }, "op", rec)
			assert.Equal(t, err, tt.err)
			assert.Equal(t, rec.calls, 1)
			assert.Equal(t, rec.operation, "op")
			assert.Equal(t, rec.success, tt.success)
		})
	}
}

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
