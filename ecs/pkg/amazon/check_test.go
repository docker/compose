package amazon

import (
	"testing"

	"gotest.tools/assert"
)

func TestInvalidNetworkMode(t *testing.T) {
	project := load(t, "testdata/invalid_network_mode.yaml")
	err := Check(project)
	assert.Error(t, err[0], "'network_mode' \"bridge\" is not supported")
}
