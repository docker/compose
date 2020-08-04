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
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestSingleError(t *testing.T) {
	var err *Error
	err = Append(err, errors.New("error"))
	assert.Assert(t, cmp.Len(err.WrappedErrors(), 1))
}

func TestGoError(t *testing.T) {
	var err error
	result := Append(err, errors.New("error"))
	assert.Assert(t, cmp.Len(result.WrappedErrors(), 1))
}

func TestMultiError(t *testing.T) {
	var err *Error
	err = Append(err,
		errors.New("first"),
		errors.New("second"),
	)
	assert.Assert(t, cmp.Len(err.WrappedErrors(), 2))
	assert.Error(t, err, "Error: first\nError: second")
}

func TestUnwrap(t *testing.T) {
	var err *Error
	assert.NilError(t, errors.Unwrap(err))

	err = Append(err, errors.New("first"))
	e := errors.Unwrap(err)
	assert.Error(t, e, "first")
}

func TestErrorOrNil(t *testing.T) {
	var err *Error
	assert.NilError(t, err.ErrorOrNil())

	err = Append(err, errors.New("error"))
	e := err.ErrorOrNil()
	assert.Error(t, e, "error")
}
