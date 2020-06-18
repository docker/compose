package azure

import (
	"testing"

	"github.com/stretchr/testify/suite"

	. "github.com/onsi/gomega"
)

type BackendSuiteTest struct {
	suite.Suite
}

func (suite *BackendSuiteTest) TestGetContainerName() {
	group, container := getGroupAndContainerName("docker1234")
	Expect(group).To(Equal("docker1234"))
	Expect(container).To(Equal(singleContainerName))

	group, container = getGroupAndContainerName("compose_service1")
	Expect(group).To(Equal("compose"))
	Expect(container).To(Equal("service1"))

	group, container = getGroupAndContainerName("compose_stack_service1")
	Expect(group).To(Equal("compose_stack"))
	Expect(container).To(Equal("service1"))
}

func TestBackendSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(BackendSuiteTest))
}
