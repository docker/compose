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

	"github.com/stretchr/testify/assert"
)

func TestSingleError(t *testing.T) {
	var err *Error
	err = Append(err, errors.New("error"))
	assert.Equal(t, 1, len(err.WrappedErrors()))
}

func TestGoError(t *testing.T) {
	var err error
	result := Append(err, errors.New("error"))
	assert.Equal(t, 1, len(result.WrappedErrors()))
}

func TestMultiError(t *testing.T) {
	var err *Error
	err = Append(err,
		errors.New("first"),
		errors.New("second"),
	)
	assert.Equal(t, 2, len(err.WrappedErrors()))
	assert.Equal(t, "Error: first\nError: second", err.Error())
}

func TestUnwrap(t *testing.T) {
	var err *Error
	assert.Equal(t, nil, errors.Unwrap(err))

	err = Append(err, errors.New("first"))
	e := errors.Unwrap(err)
	assert.Equal(t, "first", e.Error())
}

func TestErrorOrNil(t *testing.T) {
	var err *Error
	assert.Equal(t, nil, err.ErrorOrNil())

	err = Append(err, errors.New("error"))
	e := err.ErrorOrNil()
	assert.Equal(t, "error", e.Error())
}
