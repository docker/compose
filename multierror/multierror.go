/*
   Copyright 2020 Docker, Inc.

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

package multierror

import (
	"strings"

	"github.com/hashicorp/go-multierror"
)

// Error wraps a multierror.Error and defines a default
// formatting function that fits cli needs
type Error struct {
	err *multierror.Error
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	e.err.ErrorFormat = listErrorFunc
	return e.err.Error()
}

// WrappedErrors returns the list of errors that this Error is wrapping.
// It is an implementation of the errwrap.Wrapper interface so that
// multierror.Error can be used with that library.
//
// This method is not safe to be called concurrently and is no different
// than accessing the Errors field directly. It is implemented only to
// satisfy the errwrap.Wrapper interface.
func (e *Error) WrappedErrors() []error {
	return e.err.WrappedErrors()
}

// Unwrap returns an error from Error (or nil if there are no errors)
func (e *Error) Unwrap() error {
	if e == nil || e.err == nil {
		return nil
	}
	return e.err.Unwrap()
}

// ErrorOrNil returns an error interface if this Error represents
// a list of errors, or returns nil if the list of errors is empty. This
// function is useful at the end of accumulation to make sure that the value
// returned represents the existence of errors.
func (e *Error) ErrorOrNil() error {
	if e == nil || e.err == nil {
		return nil
	}
	if len(e.err.Errors) == 0 {
		return nil
	}

	return e
}

// Append adds an error to a multierror, if err is
// not a multierror it will be converted to one
func Append(err error, errs ...error) *Error {
	switch err := err.(type) {
	case *Error:
		if err == nil {
			err = new(Error)
		}
		for _, e := range errs {
			err.err = multierror.Append(err.err, e)
		}
		return err
	default:
		newErrs := make([]error, 0, len(errs)+1)
		if err != nil {
			newErrs = append(newErrs, err)
		}
		newErrs = append(newErrs, errs...)

		return Append(&Error{}, newErrs...)
	}
}

func listErrorFunc(errs []error) string {
	if len(errs) == 1 {
		return "Error: " + errs[0].Error()
	}

	messages := make([]string, len(errs))

	for i, err := range errs {
		messages[i] = "Error: " + err.Error()
	}

	return strings.Join(messages, "\n")
}
