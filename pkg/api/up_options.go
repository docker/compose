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

// IncompatibleOptionsError reports options, or option values, that were
// requested together but cannot be honored together. It is returned, before any side effect, by
// UpOptions.Validate and therefore by Compose.Up.
//
// It matches ErrIncompatibleOptions with errors.Is, and callers needing the
// details can retrieve it with errors.As:
//
//	var incompatible *api.IncompatibleOptionsError
//	if errors.As(err, &incompatible) {
//		log.Printf("conflict between %v: %s", incompatible.Options, incompatible.Reason)
//	}
type IncompatibleOptionsError struct {
	// Options names the conflicting option fields, as Go selectors relative to
	// the options type that was validated (e.g. "Start.Wait").
	Options []string
	// Reason explains why the options, as set, cannot be honored together.
	Reason string
}

func (e *IncompatibleOptionsError) Error() string {
	return fmt.Sprintf("%s (%s): %s", ErrIncompatibleOptions, strings.Join(e.Options, ", "), e.Reason)
}

// Unwrap makes errors.Is(err, ErrIncompatibleOptions) hold.
func (e *IncompatibleOptionsError) Unwrap() error {
	return ErrIncompatibleOptions
}

func incompatible(reason string, options ...string) error {
	return &IncompatibleOptionsError{Options: options, Reason: reason}
}

// Validate checks that the options requested for Up can be honored together.
// It is a pure function, called by Up before it does anything, and can also be
// called by integrators to fail early. It returns nil, or an error matching
// ErrIncompatibleOptions describing every conflict found.
//
// Up has two modes, selected by Start.Attach: detached (nil) returns once the
// containers are started, interactive (set) keeps running a foreground session.
// Validate only checks the combinations listed below: Wait with Attach, and
// OnExit, ExitCodeFrom or Watch without Attach. Other mode-specific fields
// (e.g. Services, AttachTo, NavigationMenu) are not checked and may be ignored
// in the mode they do not apply to.
//
// With a single conflict the error is an *IncompatibleOptionsError. With
// several, the conflicts are reported together as an error joined with
// errors.Join: errors.Is(err, ErrIncompatibleOptions) still holds, but
// errors.As to *IncompatibleOptionsError only yields the first conflict. To
// inspect all of them, walk the joined error through its Unwrap() []error
// method.
//
// The compose CLI refuses the same combinations, with messages naming its
// flags; Validate is the safety net for programs using the library directly.
func (o UpOptions) Validate() error {
	var errs []error
	errs = append(errs, validateWaitIsDetached(o.Start)...)
	errs = append(errs, validateDetachedHasNoSession(o.Start)...)
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errors.Join(errs...)
	}
}

// validateWaitIsDetached: Wait blocks until the services are running|healthy
// and then returns, which only happens in detached mode. In an interactive
// session that check would be silently dropped.
// CLI: --wait implies --detach and is incompatible with --attach and
// --attach-dependencies.
func validateWaitIsDetached(s StartOptions) []error {
	if s.Wait && s.Attach != nil {
		return []error{incompatible("Wait returns once services are running or healthy, which is detached mode, while Attach starts an interactive session",
			"Start.Wait", "Start.Attach")}
	}
	return nil
}

// validateDetachedHasNoSession: the cascade policy, the exit code selection
// and watch are all features of the interactive session, they have no effect
// when Attach is nil. Every such option set is reported. When Wait is set, it
// is the reason why the session cannot be started, so it is part of the conflict.
// CLI: --detach is incompatible with --abort-on-container-exit,
// --abort-on-container-failure, --exit-code-from (which implies
// --abort-on-container-exit) and --watch.
func validateDetachedHasNoSession(s StartOptions) []error {
	if s.Attach != nil {
		return nil
	}
	reason := "Attach is nil (detached mode), so no interactive session runs to honor "
	options := []string{"Start.Attach"}
	if s.Wait {
		reason = "Wait implies detached mode and Attach is nil, so no interactive session runs to honor "
		options = []string{"Start.Wait", "Start.Attach"}
	}
	conflict := func(field string) error {
		return incompatible(reason+field, append(slices.Clone(options), "Start."+field)...)
	}
	var errs []error
	if s.OnExit != CascadeIgnore {
		errs = append(errs, conflict("OnExit"))
	}
	if s.ExitCodeFrom != "" {
		errs = append(errs, conflict("ExitCodeFrom"))
	}
	if s.Watch {
		errs = append(errs, conflict("Watch"))
	}
	return errs
}
