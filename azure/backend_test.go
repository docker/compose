package azure

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/stretchr/testify/suite"

	. "github.com/onsi/gomega"

	"github.com/docker/api/azure/convert"
	"github.com/docker/api/containers"
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

func TestContainerGroupToContainer(t *testing.T) {
	myContainerGroup := containerinstance.ContainerGroup{
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			IPAddress: &containerinstance.IPAddress{
				Ports: &[]containerinstance.Port{{
					Port: to.Int32Ptr(80),
				}},
				IP: to.StringPtr("42.42.42.42"),
			},
		},
	}
	myContainer := containerinstance.Container{
		Name: to.StringPtr("myContainerID"),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:   to.StringPtr("sha256:666"),
			Command: to.StringSlicePtr([]string{"mycommand"}),
			Ports: &[]containerinstance.ContainerPort{{
				Port: to.Int32Ptr(80),
			}},
			EnvironmentVariables: nil,
			InstanceView: &containerinstance.ContainerPropertiesInstanceView{
				RestartCount: nil,
				CurrentState: &containerinstance.ContainerState{
					State: to.StringPtr("Running"),
				},
			},
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					MemoryInGB: to.Float64Ptr(9),
				},
			},
		},
	}

	var expectedContainer = containers.Container{
		ID:          "myContainerID",
		Status:      "Running",
		Image:       "sha256:666",
		Command:     "mycommand",
		MemoryLimit: 9,
		Ports: []containers.Port{{
			HostPort:      uint32(80),
			ContainerPort: uint32(80),
			Protocol:      "tcp",
			HostIP:        "42.42.42.42",
		}},
	}

	container, err := convert.ContainerGroupToContainer("myContainerID", myContainerGroup, myContainer)
	Expect(err).To(BeNil())
	Expect(container).To(Equal(expectedContainer))
}
