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
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestIsNotFound(t *testing.T) {
	err := errors.Wrap(ErrNotFound, `object "name"`)
	assert.True(t, IsNotFoundError(err))

	assert.False(t, IsNotFoundError(errors.New("another error")))
}

func TestIsAlreadyExists(t *testing.T) {
	err := errors.Wrap(ErrAlreadyExists, `object "name"`)
	assert.True(t, IsAlreadyExistsError(err))

	assert.False(t, IsAlreadyExistsError(errors.New("another error")))
}

func TestIsForbidden(t *testing.T) {
	err := errors.Wrap(ErrForbidden, `object "name"`)
	assert.True(t, IsForbiddenError(err))

	assert.False(t, IsForbiddenError(errors.New("another error")))
}

func TestIsUnknown(t *testing.T) {
	err := errors.Wrap(ErrUnknown, `object "name"`)
	assert.True(t, IsUnknownError(err))

	assert.False(t, IsUnknownError(errors.New("another error")))
}
