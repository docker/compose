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
	"github.com/pkg/errors"
)

const (
	//ExitCodeLoginRequired exit code when command cannot execute because it requires cloud login
	// This will be used by VSCode to detect when creating context if the user needs to login first
	ExitCodeLoginRequired = 5
)

var (
	// ErrNotFound is returned when an object is not found
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists is returned when an object already exists
	ErrAlreadyExists = errors.New("already exists")
	// ErrForbidden is returned when an operation is not permitted
	ErrForbidden = errors.New("forbidden")
	// ErrUnknown is returned when the error type is unmapped
	ErrUnknown = errors.New("unknown")
	// ErrLoginFailed is returned when login failed
	ErrLoginFailed = errors.New("login failed")
	// ErrLoginRequired is returned when login is required for a specific action
	ErrLoginRequired = errors.New("login required")
	// ErrNotImplemented is returned when a backend doesn't implement
	// an action
	ErrNotImplemented = errors.New("not implemented")
	// ErrUnsupportedFlag is returned when a backend doesn't support a flag
	ErrUnsupportedFlag = errors.New("unsupported flag")
	// ErrCanceled is returned when the command was canceled by user
	ErrCanceled = errors.New("canceled")
	// ErrParsingFailed is returned when a string cannot be parsed
	ErrParsingFailed = errors.New("parsing failed")
	// ErrWrongContextType is returned when the caller tries to get a context
	// with the wrong type
	ErrWrongContextType = errors.New("wrong context type")
)

// IsNotFoundError returns true if the unwrapped error is ErrNotFound
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExistsError returns true if the unwrapped error is ErrAlreadyExists
func IsAlreadyExistsError(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsForbiddenError returns true if the unwrapped error is ErrForbidden
func IsForbiddenError(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsUnknownError returns true if the unwrapped error is ErrUnknown
func IsUnknownError(err error) bool {
	return errors.Is(err, ErrUnknown)
}

// IsErrUnsupportedFlag returns true if the unwrapped error is ErrUnsupportedFlag
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
