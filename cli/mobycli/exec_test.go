package mobycli

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/docker/api/context/store"
	"github.com/docker/api/tests/framework"
)

type MobyExecSuite struct {
	framework.CliSuite
}

func (sut *MobyExecSuite) TestDelegateContextTypeToMoby() {

	isDelegated := func(val string) bool {
		for _, ctx := range delegatedContextTypes {
			if ctx == val {
				return true
			}
		}
		return false
	}

	allCtx := []string{store.AciContextType, store.EcsContextType, store.AwsContextType, store.DefaultContextType}
	for _, ctx := range allCtx {
		if isDelegated(ctx) {
			Expect(mustDelegateToMoby(ctx)).To(BeTrue())
			continue
		}
		Expect(mustDelegateToMoby(ctx)).To(BeFalse())
	}
}

func TestExec(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(MobyExecSuite))
}
