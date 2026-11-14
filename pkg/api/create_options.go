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
	"slices"
	"strings"
)

// InvalidOptionError reports an option set to a value it does not accept. It is
// returned, before any side effect, by CreateOptions.Validate and therefore by
// Compose.Create and Compose.Up.
//
// It matches ErrInvalidOption with errors.Is, and callers needing the details
// can retrieve it with errors.As.
type InvalidOptionError struct {
	// Option names the option field, as a Go selector relative to the options
	// type that was validated (e.g. "Recreate").
	Option string
	// Value is the value that was set.
	Value string
	// Allowed lists the values the option accepts, when it is a closed set.
	Allowed []string
	// Reason explains what is wrong when there is no value to blame, for
	// instance a required option left unset. It replaces Value and Allowed in
	// the message.
	Reason string
}

func (e *InvalidOptionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s (%s): %s", ErrInvalidOption, e.Option, e.Reason)
	}
	return fmt.Sprintf("%s (%s): %q is not one of %s", ErrInvalidOption, e.Option, e.Value, strings.Join(e.Allowed, ", "))
}

// Unwrap makes errors.Is(err, ErrInvalidOption) hold.
func (e *InvalidOptionError) Unwrap() error {
	return ErrInvalidOption
}

// Validate checks that the values of the options are among the ones they
// accept: Recreate and RecreateDependencies must be empty (which means
// RecreateDiverged) or one of the Recreate* constants. It is a pure function,
// called by Create and Up before they do anything, and can also be called by
// integrators to fail early.
//
// It returns nil, or an error matching ErrInvalidOption. With several invalid
// options, the errors are joined with errors.Join: errors.Is still holds, but
// errors.As to *InvalidOptionError only yields the first one.
func (o CreateOptions) Validate() error {
	var errs []error
	errs = append(errs, validateRecreatePolicy("Recreate", o.Recreate)...)
	errs = append(errs, validateRecreatePolicy("RecreateDependencies", o.RecreateDependencies)...)
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errors.Join(errs...)
	}
}

func validateRecreatePolicy(option, policy string) []error {
	allowed := []string{RecreateDiverged, RecreateForce, RecreateNever}
	if policy == "" || slices.Contains(allowed, policy) {
		return nil
	}
	return []error{&InvalidOptionError{Option: option, Value: policy, Allowed: allowed}}
}
