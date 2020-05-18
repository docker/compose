package amazon

import (
	"testing"

	"gotest.tools/assert"
)

func TestInvalidNetworkMode(t *testing.T) {
	project := load(t, "testdata/invalid_network_mode.yaml")
	err := Validate(project)
	assert.Error(t, err, "ECS do not support NetworkMode \"bridge\"")
}
