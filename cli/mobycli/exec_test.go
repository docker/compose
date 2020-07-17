package mobycli

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/tests/framework"
)

type MobyExecSuite struct {
	framework.CliSuite
}

func (sut *MobyExecSuite) TestDelegateContextTypeToMoby() {
	Expect(mustDelegateToMoby("moby")).To(BeTrue())
	Expect(mustDelegateToMoby("aws")).To(BeFalse())
	Expect(mustDelegateToMoby("aci")).To(BeFalse())
}

func TestExec(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(MobyExecSuite))
}
