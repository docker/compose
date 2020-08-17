package commands

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnknownCommand(t *testing.T) {
	root := NewRootCmd(nil)
	_, _, err := root.Find([]string{"unknown_command"})
	assert.Error(t, err, "unknown command \"unknown_command\" for \"ecs\"")
}
