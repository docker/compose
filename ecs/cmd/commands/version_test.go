package commands

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestVersion(t *testing.T) {
	root := NewRootCmd(nil)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	root.Execute()
	assert.Check(t, strings.Contains(out.String(), Version))
}
