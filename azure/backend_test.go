package azure

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestGetContainerName ensures we can read container group name / container name from a containerID
func TestGetContainerName(t *testing.T) {
	RegisterTestingT(t)

	group, container := getGrouNameContainername("docker1234")
	Expect(group).To(Equal("docker1234"))
	Expect(container).To(Equal(singleContainerName))

	group, container = getGrouNameContainername("compose_service1")
	Expect(group).To(Equal("compose"))
	Expect(container).To(Equal("service1"))

	group, container = getGrouNameContainername("compose_stack_service1")
	Expect(group).To(Equal("compose_stack"))
	Expect(container).To(Equal("service1"))
}
