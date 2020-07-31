package aci

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGetLinesWritten(t *testing.T) {
	assert.Equal(t, 0, getBacktrackLines([]string{"Hello"}, 10))
	assert.Equal(t, 3, getBacktrackLines([]string{"Hello", "world"}, 2))
}
