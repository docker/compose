/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package errdefs

import (
	"github.com/pkg/errors"
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
