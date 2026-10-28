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
	"testing"

	"gotest.tools/v3/assert"
)

func TestCreateOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options CreateOptions
		// invalid lists the options expected to be rejected, empty when valid
		invalid []string
	}{
		{name: "zero value is valid", options: CreateOptions{}},
		{name: "diverged", options: CreateOptions{Recreate: RecreateDiverged, RecreateDependencies: RecreateDiverged}},
		{name: "force", options: CreateOptions{Recreate: RecreateForce, RecreateDependencies: RecreateForce}},
		{name: "never", options: CreateOptions{Recreate: RecreateNever, RecreateDependencies: RecreateNever}},
		{name: "unknown recreate", options: CreateOptions{Recreate: "forced"}, invalid: []string{"Recreate"}},
		{name: "unknown dependencies recreate", options: CreateOptions{RecreateDependencies: "always"}, invalid: []string{"RecreateDependencies"}},
		{name: "case matters", options: CreateOptions{Recreate: "Force"}, invalid: []string{"Recreate"}},
		{name: "both unknown", options: CreateOptions{Recreate: "x", RecreateDependencies: "y"}, invalid: []string{"Recreate", "RecreateDependencies"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if len(tt.invalid) == 0 {
				assert.NilError(t, err)
				return
			}
			assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)
			assert.Assert(t, IsErrInvalidOption(err))

			// every invalid option is reported, whether the error is a single
			// *InvalidOptionError or several of them joined
			var reported []string
			var walk func(error)
			walk = func(e error) {
				if joined, ok := e.(interface{ Unwrap() []error }); ok {
					for _, inner := range joined.Unwrap() {
						walk(inner)
					}
					return
				}
				var invalid *InvalidOptionError
				if errors.As(e, &invalid) {
					reported = append(reported, invalid.Option)
				}
			}
			walk(err)
			assert.DeepEqual(t, reported, tt.invalid)
		})
	}
}

func TestInvalidOptionErrorMessage(t *testing.T) {
	err := CreateOptions{Recreate: "forced"}.Validate()
	assert.Error(t, err, `invalid option (Recreate): "forced" is not one of diverged, force, never`)
}

func TestUpOptionsValidateReportsInvalidCreateOptions(t *testing.T) {
	err := UpOptions{Create: CreateOptions{Recreate: "forced"}}.Validate()
	assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)

	// invalid values and incompatible combinations are reported together
	err = UpOptions{Create: CreateOptions{Recreate: "forced"}, Start: StartOptions{Wait: true, Attach: nopConsumer{}}}.Validate()
	assert.Assert(t, errors.Is(err, ErrInvalidOption), "got %v", err)
	assert.Assert(t, errors.Is(err, ErrIncompatibleOptions), "got %v", err)
}
