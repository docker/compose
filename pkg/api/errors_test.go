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
	"testing"

	"github.com/pkg/errors"
	"gotest.tools/v3/assert"
)

func TestIsNotFound(t *testing.T) {
	err := errors.Wrap(ErrNotFound, `object "name"`)
	assert.Assert(t, IsNotFoundError(err))

	assert.Assert(t, !IsNotFoundError(errors.New("another error")))
}

func TestIsAlreadyExists(t *testing.T) {
	err := errors.Wrap(ErrAlreadyExists, `object "name"`)
	assert.Assert(t, IsAlreadyExistsError(err))

	assert.Assert(t, !IsAlreadyExistsError(errors.New("another error")))
}

func TestIsForbidden(t *testing.T) {
	err := errors.Wrap(ErrForbidden, `object "name"`)
	assert.Assert(t, IsForbiddenError(err))

	assert.Assert(t, !IsForbiddenError(errors.New("another error")))
}

func TestIsUnknown(t *testing.T) {
	err := errors.Wrap(ErrUnknown, `object "name"`)
	assert.Assert(t, IsUnknownError(err))

	assert.Assert(t, !IsUnknownError(errors.New("another error")))
}
