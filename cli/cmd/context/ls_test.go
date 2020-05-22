package context

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/golden"

	"github.com/docker/api/tests/framework"
)

type ContextSuite struct {
	framework.CliSuite
}

func (sut *ContextSuite) TestLs() {
	err := runList(sut.Context())
	require.Nil(sut.T(), err)
	golden.Assert(sut.T(), sut.GetStdOut(), "ls-out.golden")
}

func TestPs(t *testing.T) {
	suite.Run(t, new(ContextSuite))
}
