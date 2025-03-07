package transform

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestReplace(t *testing.T) {
	out, err := Replace([]byte(`services:
  test:
    extends:
      # some comment before
      file: foo.yaml
      # some comment after
      service: foo
`), "test", "REPLACED")
	assert.NilError(t, err)
	assert.Equal(t, string(out), `services:
  test:
    extends:
      # some comment before
      file: REPLACED
      # some comment after
      service: foo
`)
}
