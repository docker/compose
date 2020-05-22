package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/golden"

	_ "github.com/docker/api/example"
	"github.com/docker/api/tests/framework"
)

type PsSuite struct {
	framework.CliSuite
}

func (sut *PsSuite) TestPs() {
	opts := psOpts{
		quiet: false,
	}

	err := runPs(sut.Context(), opts)
	require.Nil(sut.T(), err)

	golden.Assert(sut.T(), sut.GetStdOut(), "ps-out.golden")
}

func (sut *PsSuite) TestPsQuiet() {
	opts := psOpts{
		quiet: true,
	}

	err := runPs(sut.Context(), opts)
	require.Nil(sut.T(), err)

	golden.Assert(sut.T(), sut.GetStdOut(), "ps-out-quiet.golden")
}

func TestPs(t *testing.T) {
	suite.Run(t, new(PsSuite))
}
