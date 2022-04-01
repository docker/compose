/*
   Copyright 2020 The Compose Specification Authors.

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

package errdefs

import "errors"

var (
	// ErrNotFound is returned when an object is not found
	ErrNotFound = errors.New("not found")

	// ErrInvalid is returned when a compose project is invalid
	ErrInvalid = errors.New("invalid compose project")

	// ErrUnsupported is returned when a compose project uses an unsupported attribute
	ErrUnsupported = errors.New("unsupported attribute")

	// ErrIncompatible is returned when a compose project uses an incompatible attribute
	ErrIncompatible = errors.New("incompatible attribute")
)

// IsNotFoundError returns true if the unwrapped error is ErrNotFound
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsInvalidError returns true if the unwrapped error is ErrInvalid
func IsInvalidError(err error) bool {
	return errors.Is(err, ErrInvalid)
}

// IsUnsupportedError returns true if the unwrapped error is ErrUnsupported
func IsUnsupportedError(err error) bool {
	return errors.Is(err, ErrUnsupported)
}

// IsUnsupportedError returns true if the unwrapped error is ErrIncompatible
func IsIncompatibleError(err error) bool {
	return errors.Is(err, ErrIncompatible)
}
