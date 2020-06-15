package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/golden"

	_ "github.com/docker/api/example"
	"github.com/docker/api/tests/framework"
)

type InspectSuite struct {
	framework.CliSuite
}

func (sut *InspectSuite) TestInspectId() {
	err := runInspect(sut.Context(), "id")
	require.Nil(sut.T(), err)
	golden.Assert(sut.T(), sut.GetStdOut(), "inspect-out-id.golden")
}

func TestInspect(t *testing.T) {
	suite.Run(t, new(InspectSuite))
}
