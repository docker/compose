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
