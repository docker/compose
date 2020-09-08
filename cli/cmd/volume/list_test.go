package volume

import (
	"bytes"
	"github.com/docker/compose-cli/api/volumes"
	"gotest.tools/v3/golden"
	"testing"
)

func TestPrintList(t *testing.T) {
	secrets := []volumes.Volume{
		{
			ID:          "volume@123",
			Name:        "123",
			Description: "volume 123",
		},
	}
	out := &bytes.Buffer{}
	printList(out, secrets)
	golden.Assert(t, out.String(), "volumes-out.golden")
}

