package context

import (
	"context"
	"testing"

	"github.com/docker/api/context/store"

	_ "github.com/docker/api/example"
	"github.com/docker/api/tests/framework"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
)

type PsSuite struct {
	framework.CliSuite
}

func (sut *PsSuite) TestCreateContextDataMoby() {
	data, description, err := getContextData(context.TODO(), "moby", AciCreateOpts{})
	Expect(err).To(BeNil())
	Expect(data).To(Equal(store.MobyContext{}))
	Expect(description).To(Equal(""))
}

func (sut *PsSuite) TestErrorOnUnknownContextType() {
	_, _, err := getContextData(context.TODO(), "foo", AciCreateOpts{})
	Expect(err).To(MatchError("incorrect context type foo, must be one of (aci | moby | docker)"))
}

func TestPs(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(PsSuite))
}
