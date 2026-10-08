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

package api

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

type nopConsumer struct{}

func (nopConsumer) Log(string, string)    {}
func (nopConsumer) Err(string, string)    {}
func (nopConsumer) Status(string, string) {}

func TestUpOptionsValidate(t *testing.T) {
	attached := nopConsumer{}

	tests := []struct {
		name  string
		start StartOptions
		// conflict lists the expected conflicting options, empty when valid
		conflict []string
	}{
		// valid combinations, closest to each rule
		{name: "zero value is detached and valid", start: StartOptions{}},
		{name: "detached with services and attach targets", start: StartOptions{Services: []string{"web"}, AttachTo: []string{"web"}}},
		{name: "detached wait", start: StartOptions{Wait: true}},
		{name: "detached wait with timeout", start: StartOptions{Wait: true, WaitTimeout: time.Minute}},
		{name: "detached wait timeout without wait", start: StartOptions{WaitTimeout: time.Minute}},
		{name: "interactive", start: StartOptions{Attach: attached}},
		{name: "interactive with menu", start: StartOptions{Attach: attached, NavigationMenu: true}},
		{name: "interactive cascade stop", start: StartOptions{Attach: attached, OnExit: CascadeStop}},
		{name: "interactive cascade fail", start: StartOptions{Attach: attached, OnExit: CascadeFail}},
		{name: "interactive exit code from", start: StartOptions{Attach: attached, ExitCodeFrom: "web"}},
		{name: "interactive exit code from with cascade", start: StartOptions{Attach: attached, OnExit: CascadeStop, ExitCodeFrom: "web"}},
		{name: "interactive watch", start: StartOptions{Attach: attached, Watch: true}},
		{name: "interactive everything", start: StartOptions{Attach: attached, AttachTo: []string{"web"}, OnExit: CascadeFail, ExitCodeFrom: "web", Watch: true, NavigationMenu: true}},

		// wait is detached
		{name: "wait with attach", start: StartOptions{Wait: true, Attach: attached}, conflict: []string{"Start.Wait", "Start.Attach"}},
		{name: "wait with attach and timeout", start: StartOptions{Wait: true, WaitTimeout: time.Second, Attach: attached}, conflict: []string{"Start.Wait", "Start.Attach"}},

		// session-only options require a session
		{name: "detached cascade stop", start: StartOptions{OnExit: CascadeStop}, conflict: []string{"Start.Attach", "Start.OnExit"}},
		{name: "detached cascade fail", start: StartOptions{OnExit: CascadeFail}, conflict: []string{"Start.Attach", "Start.OnExit"}},
		{name: "detached exit code from", start: StartOptions{ExitCodeFrom: "web"}, conflict: []string{"Start.Attach", "Start.ExitCodeFrom"}},
		{name: "detached watch", start: StartOptions{Watch: true}, conflict: []string{"Start.Attach", "Start.Watch"}},
		{name: "wait with cascade", start: StartOptions{Wait: true, OnExit: CascadeStop}, conflict: []string{"Start.Wait", "Start.Attach", "Start.OnExit"}},
		{name: "wait with exit code from", start: StartOptions{Wait: true, ExitCodeFrom: "web"}, conflict: []string{"Start.Wait", "Start.Attach", "Start.ExitCodeFrom"}},
		{name: "wait with watch", start: StartOptions{Wait: true, Watch: true}, conflict: []string{"Start.Wait", "Start.Attach", "Start.Watch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpOptions{Start: tt.start}.Validate()
			if tt.conflict == nil {
				assert.NilError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrIncompatibleOptions)
			assert.Assert(t, IsErrIncompatibleOptions(err))

			var incompatible *IncompatibleOptionsError
			assert.Assert(t, errors.As(err, &incompatible), "expected *IncompatibleOptionsError, got %T", err)
			assert.DeepEqual(t, incompatible.Options, tt.conflict)
			assert.Assert(t, incompatible.Reason != "")
		})
	}
}

func TestUpOptionsValidateIgnoresCreateOptions(t *testing.T) {
	// Create options are not subject to validation: the compose CLI rules
	// about recreation or build only have a meaning at flag level.
	err := UpOptions{Create: CreateOptions{Recreate: RecreateNever, RecreateDependencies: RecreateForce}}.Validate()
	assert.NilError(t, err)
}

func TestIncompatibleOptionsError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &IncompatibleOptionsError{
		Options: []string{"A", "B"},
		Reason:  "because",
	})
	assert.ErrorIs(t, err, ErrIncompatibleOptions)
	assert.Assert(t, IsErrIncompatibleOptions(err))
	assert.Assert(t, !IsErrIncompatibleOptions(ErrNotFound))
	var incompatible *IncompatibleOptionsError
	assert.Assert(t, errors.As(err, &incompatible))
	assert.Equal(t, incompatible.Error(), "incompatible options (A, B): because")
}

// conflictsOf returns the options of each conflict reported by err, in order.
func conflictsOf(t *testing.T, err error) [][]string {
	t.Helper()
	errs := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	}
	var all [][]string
	for _, e := range errs {
		var incompatible *IncompatibleOptionsError
		assert.Assert(t, errors.As(e, &incompatible), "expected *IncompatibleOptionsError, got %T", e)
		all = append(all, incompatible.Options)
	}
	return all
}

func TestUpOptionsValidateReportsAllConflicts(t *testing.T) {
	attached := nopConsumer{}

	tests := []struct {
		name  string
		start StartOptions
		want  [][]string
	}{
		{
			name:  "two session options",
			start: StartOptions{OnExit: CascadeStop, ExitCodeFrom: "web"},
			want:  [][]string{{"Start.Attach", "Start.OnExit"}, {"Start.Attach", "Start.ExitCodeFrom"}},
		},
		{
			name:  "exit code from and watch",
			start: StartOptions{ExitCodeFrom: "web", Watch: true},
			want:  [][]string{{"Start.Attach", "Start.ExitCodeFrom"}, {"Start.Attach", "Start.Watch"}},
		},
		{
			name:  "all session options",
			start: StartOptions{OnExit: CascadeFail, ExitCodeFrom: "web", Watch: true},
			want:  [][]string{{"Start.Attach", "Start.OnExit"}, {"Start.Attach", "Start.ExitCodeFrom"}, {"Start.Attach", "Start.Watch"}},
		},
		{
			name:  "wait with all session options",
			start: StartOptions{Wait: true, OnExit: CascadeStop, ExitCodeFrom: "web", Watch: true},
			want: [][]string{
				{"Start.Wait", "Start.Attach", "Start.OnExit"},
				{"Start.Wait", "Start.Attach", "Start.ExitCodeFrom"},
				{"Start.Wait", "Start.Attach", "Start.Watch"},
			},
		},
		{
			name:  "single session option is not wrapped",
			start: StartOptions{Watch: true},
			want:  [][]string{{"Start.Attach", "Start.Watch"}},
		},
		{
			// the session options are fine once Attach is set, only Wait conflicts
			name:  "wait with attach and session options",
			start: StartOptions{Wait: true, Attach: attached, OnExit: CascadeStop, ExitCodeFrom: "web", Watch: true},
			want:  [][]string{{"Start.Wait", "Start.Attach"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpOptions{Start: tt.start}.Validate()
			assert.ErrorIs(t, err, ErrIncompatibleOptions)
			assert.Assert(t, IsErrIncompatibleOptions(err))
			assert.DeepEqual(t, conflictsOf(t, err), tt.want)

			// errors.As on the combined error yields the first conflict
			var incompatible *IncompatibleOptionsError
			assert.Assert(t, errors.As(err, &incompatible))
			assert.DeepEqual(t, incompatible.Options, tt.want[0])
		})
	}
}
