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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// Up must refuse incompatible options before any side effect: the strict mock
// client fails the test on any engine call, and no progress event may be
// emitted either.
func TestUpRejectsIncompatibleOptionsBeforeAnySideEffect(t *testing.T) {
	tests := []struct {
		name    string
		options api.UpOptions
		want    []string
	}{
		{
			name:    "wait with attach",
			options: api.UpOptions{Start: api.StartOptions{Wait: true, Attach: &testLogConsumer{}}},
			want:    []string{"Start.Wait", "Start.Attach"},
		},
		{
			name:    "detached with cascade",
			options: api.UpOptions{Start: api.StartOptions{OnExit: api.CascadeStop}},
			want:    []string{"Start.Attach", "Start.OnExit"},
		},
		{
			name:    "detached with exit code from",
			options: api.UpOptions{Start: api.StartOptions{ExitCodeFrom: "web"}},
			want:    []string{"Start.Attach", "Start.ExitCodeFrom"},
		},
		{
			name:    "detached with watch",
			options: api.UpOptions{Start: api.StartOptions{Watch: true}},
			want:    []string{"Start.Attach", "Start.Watch"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, rec := newStartTestService(t)
			project := &types.Project{Name: "prj", Services: types.Services{"web": {Name: "web"}}}

			err := svc.Up(t.Context(), project, tt.options)

			assert.ErrorIs(t, err, api.ErrIncompatibleOptions)
			var incompatible *api.IncompatibleOptionsError
			assert.Assert(t, errors.As(err, &incompatible), "expected *api.IncompatibleOptionsError, got %T", err)
			assert.DeepEqual(t, incompatible.Options, tt.want)
			assert.Equal(t, len(rec.summary()), 0, "no progress event expected, got %v", rec.summary())
		})
	}
}
