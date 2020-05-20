package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultContext(t *testing.T) {
	s, err := dockerDefaultContext()
	assert.Nil(t, err)
	assert.Equal(t, "default", s.Name)
}
