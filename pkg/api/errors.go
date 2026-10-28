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
)

const (
	// ExitCodeLoginRequired exit code when command cannot execute because it requires cloud login
	// This will be used by VSCode to detect when creating context if the user needs to login first
	//
	// Deprecated: Compose has no cloud login anymore and never exits with this code, it will be removed.
	ExitCodeLoginRequired = 5
)

var (
	// ErrNotFound is returned when an object is not found
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists is returned when an object already exists
	//
	// Deprecated: Compose never returns it, it will be removed.
	ErrAlreadyExists = errors.New("already exists")
	// ErrForbidden is returned when an operation is not permitted
	//
	// Deprecated: Compose never returns it, it will be removed.
	ErrForbidden = errors.New("forbidden")
	// ErrUnknown is returned when the error type is unmapped
	//
	// Deprecated: Compose never returns it, it will be removed.
	ErrUnknown = errors.New("unknown")
	// ErrNotImplemented is returned when a backend doesn't implement an action
	ErrNotImplemented = errors.New("not implemented")
	// ErrUnsupportedFlag is returned when a backend doesn't support a flag
	//
	// Deprecated: Compose never returns it, it will be removed.
	ErrUnsupportedFlag = errors.New("unsupported flag")
	// ErrCanceled is returned when the command was canceled by user
	ErrCanceled = errors.New("canceled")
	// ErrParsingFailed is returned when a string cannot be parsed
	ErrParsingFailed = errors.New("parsing failed")
	// ErrNoResources is returned when operation didn't selected any resource
	ErrNoResources = errors.New("no resources")
	// ErrIncompatibleOptions is returned when a set of options that cannot be
	// honored together is requested. See IncompatibleOptionsError.
	ErrIncompatibleOptions = errors.New("incompatible options")
	// ErrNotRunning is returned when an operation targets a service, or a
	// replica of it, that has no running container. See ServiceNotRunningError.
	ErrNotRunning = errors.New("not running")
	// ErrInvalidOption is returned when an option is set to a value it does
	// not accept. See InvalidOptionError.
	ErrInvalidOption = errors.New("invalid option")
)

// IsNotFoundError returns true if the unwrapped error is ErrNotFound
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExistsError returns true if the unwrapped error is ErrAlreadyExists
//
// Deprecated: ErrAlreadyExists is never returned, it will be removed.
func IsAlreadyExistsError(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsForbiddenError returns true if the unwrapped error is ErrForbidden
//
// Deprecated: ErrForbidden is never returned, it will be removed.
func IsForbiddenError(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsUnknownError returns true if the unwrapped error is ErrUnknown
//
// Deprecated: ErrUnknown is never returned, it will be removed.
func IsUnknownError(err error) bool {
	return errors.Is(err, ErrUnknown)
}

// IsErrUnsupportedFlag returns true if the unwrapped error is ErrUnsupportedFlag
//
// Deprecated: ErrUnsupportedFlag is never returned, it will be removed.
func IsErrUnsupportedFlag(err error) bool {
	return errors.Is(err, ErrUnsupportedFlag)
}

// IsErrNotImplemented returns true if the unwrapped error is ErrNotImplemented
func IsErrNotImplemented(err error) bool {
	return errors.Is(err, ErrNotImplemented)
}

// IsErrParsingFailed returns true if the unwrapped error is ErrParsingFailed
func IsErrParsingFailed(err error) bool {
	return errors.Is(err, ErrParsingFailed)
}

// IsErrCanceled returns true if the unwrapped error is ErrCanceled
func IsErrCanceled(err error) bool {
	return errors.Is(err, ErrCanceled)
}

// IsErrIncompatibleOptions returns true if the unwrapped error is ErrIncompatibleOptions
func IsErrIncompatibleOptions(err error) bool {
	return errors.Is(err, ErrIncompatibleOptions)
}

// ServiceNotRunningError reports that an operation which acts on one container
// of a service (exec, port, export, commit, attach…) found no matching
// container, that is no running one for most of them. It
// matches ErrNotRunning with errors.Is, and callers needing the details can
// retrieve it with errors.As.
type ServiceNotRunningError struct {
	// Service is the name of the service that was targeted.
	Service string
	// Index is the replica that was asked for, 0 when none in particular was.
	Index int
}

func (e *ServiceNotRunningError) Error() string {
	if e.Index > 0 {
		return fmt.Sprintf("service %q is not running container #%d", e.Service, e.Index)
	}
	return fmt.Sprintf("service %q is not running", e.Service)
}

// Unwrap makes errors.Is(err, ErrNotRunning) hold.
func (e *ServiceNotRunningError) Unwrap() error {
	return ErrNotRunning
}

// IsErrNotRunning returns true if the unwrapped error is ErrNotRunning
func IsErrNotRunning(err error) bool {
	return errors.Is(err, ErrNotRunning)
}

// IsErrInvalidOption returns true if the unwrapped error is ErrInvalidOption
func IsErrInvalidOption(err error) bool {
	return errors.Is(err, ErrInvalidOption)
}
