/*
   Copyright 2020 Docker, Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package convert

import (
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"

	"github.com/docker/api/containers"
	"github.com/docker/api/context/store"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ConvertTestSuite struct {
	suite.Suite
	ctx store.AciContext
}

func (suite *ConvertTestSuite) BeforeTest(suiteName, testName string) {
	suite.ctx = store.AciContext{
		SubscriptionID: "subID",
		ResourceGroup:  "rg",
		Location:       "eu",
	}
}

func (suite *ConvertTestSuite) TestProjectName() {
	project := types.Project{
		Name: "TEST",
	}
	containerGroup, err := ToContainerGroup(suite.ctx, project)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), *containerGroup.Name, "test")
}

func (suite *ConvertTestSuite) TestContainerGroupToContainer() {
	myContainerGroup := containerinstance.ContainerGroup{
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			IPAddress: &containerinstance.IPAddress{
				Ports: &[]containerinstance.Port{{
					Port: to.Int32Ptr(80),
				}},
				IP: to.StringPtr("42.42.42.42"),
			},
			OsType: "Linux",
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
					CPU:        to.Float64Ptr(3),
					MemoryInGB: to.Float64Ptr(0.1),
				},
			},
		},
	}

	var expectedContainer = containers.Container{
		ID:          "myContainerID",
		Status:      "Running",
		Image:       "sha256:666",
		Command:     "mycommand",
		CPULimit:    3,
		MemoryLimit: 107374182,
		Platform:    "Linux",
		Ports: []containers.Port{{
			HostPort:      uint32(80),
			ContainerPort: uint32(80),
			Protocol:      "tcp",
			HostIP:        "42.42.42.42",
		}},
		RestartPolicyCondition: "any",
	}

	container, err := ContainerGroupToContainer("myContainerID", myContainerGroup, myContainer)
	Expect(err).To(BeNil())
	Expect(container).To(Equal(expectedContainer))
}

func (suite *ConvertTestSuite) TestComposeContainerGroupToContainerWithDnsSideCarSide() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
			{
				Name:  "service2",
				Image: "image2",
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())
	Expect(len(*group.Containers)).To(Equal(3))

	Expect(*(*group.Containers)[0].Name).To(Equal("service1"))
	Expect(*(*group.Containers)[1].Name).To(Equal("service2"))
	Expect(*(*group.Containers)[2].Name).To(Equal(ComposeDNSSidecarName))

	Expect(*(*group.Containers)[2].Command).To(Equal([]string{"sh", "-c", "echo 127.0.0.1 service1 >> /etc/hosts;echo 127.0.0.1 service2 >> /etc/hosts;sleep infinity"}))

	Expect(*(*group.Containers)[0].Image).To(Equal("image1"))
	Expect(*(*group.Containers)[1].Image).To(Equal("image2"))
	Expect(*(*group.Containers)[2].Image).To(Equal(dnsSidecarImage))
}

func (suite *ConvertTestSuite) TestComposeSingleContainerGroupToContainerNoDnsSideCarSide() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	Expect(len(*group.Containers)).To(Equal(1))
	Expect(*(*group.Containers)[0].Name).To(Equal("service1"))
	Expect(*(*group.Containers)[0].Image).To(Equal("image1"))
}

func (suite *ConvertTestSuite) TestComposeSingleContainerGroupToContainerSpecificRestartPolicy() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					RestartPolicy: &types.RestartPolicy{
						Condition: "on-failure",
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	Expect(len(*group.Containers)).To(Equal(1))
	Expect(*(*group.Containers)[0].Name).To(Equal("service1"))
	Expect(group.RestartPolicy).To(Equal(containerinstance.OnFailure))
}

func (suite *ConvertTestSuite) TestComposeSingleContainerGroupToContainerDefaultRestartPolicy() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	Expect(len(*group.Containers)).To(Equal(1))
	Expect(*(*group.Containers)[0].Name).To(Equal("service1"))
	Expect(group.RestartPolicy).To(Equal(containerinstance.Always))
}

func (suite *ConvertTestSuite) TestComposeContainerGroupToContainerMultiplePorts() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Ports: []types.ServicePortConfig{
					{
						Published: 80,
						Target:    80,
					},
				},
			},
			{
				Name:  "service2",
				Image: "image2",
				Ports: []types.ServicePortConfig{
					{
						Published: 8080,
						Target:    8080,
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())
	Expect(len(*group.Containers)).To(Equal(3))

	container1 := (*group.Containers)[0]
	container2 := (*group.Containers)[1]
	Expect(*container1.Name).To(Equal("service1"))
	Expect(*container1.Image).To(Equal("image1"))
	portsC1 := *container1.Ports
	Expect(*portsC1[0].Port).To(Equal(int32(80)))

	Expect(*container2.Name).To(Equal("service2"))
	Expect(*container2.Image).To(Equal("image2"))
	portsC2 := *container2.Ports
	Expect(*portsC2[0].Port).To(Equal(int32(8080)))

	groupPorts := *group.IPAddress.Ports
	Expect(len(groupPorts)).To(Equal(2))
	Expect(*groupPorts[0].Port).To(Equal(int32(80)))
	Expect(*groupPorts[1].Port).To(Equal(int32(8080)))
}

func (suite *ConvertTestSuite) TestComposeContainerGroupToContainerResourceLimits() {
	_0_1Gb := 0.1 * 1024 * 1024 * 1024
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Limits: &types.Resource{
							NanoCPUs:    "0.1",
							MemoryBytes: types.UnitBytes(_0_1Gb),
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	container1 := (*group.Containers)[0]
	limits := *container1.Resources.Limits
	Expect(*limits.CPU).To(Equal(float64(0.1)))
	Expect(*limits.MemoryInGB).To(Equal(float64(0.1)))
}

func (suite *ConvertTestSuite) TestComposeContainerGroupToContainerResourceLimitsDefaults() {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Limits: &types.Resource{
							NanoCPUs:    "",
							MemoryBytes: 0,
						},
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	container1 := (*group.Containers)[0]
	limits := *container1.Resources.Limits
	Expect(*limits.CPU).To(Equal(float64(1)))
	Expect(*limits.MemoryInGB).To(Equal(float64(1)))
}

func (suite *ConvertTestSuite) TestComposeContainerGroupToContainerenvVar() {
	err := os.Setenv("key2", "value2")
	Expect(err).To(BeNil())
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Environment: types.MappingWithEquals{
					"key1": to.StringPtr("value1"),
					"key2": nil,
				},
			},
		},
	}

	group, err := ToContainerGroup(suite.ctx, project)
	Expect(err).To(BeNil())

	container1 := (*group.Containers)[0]
	envVars := *container1.EnvironmentVariables
	Expect(len(envVars)).To(Equal(2))
	Expect(envVars).To(ContainElement(containerinstance.EnvironmentVariable{Name: to.StringPtr("key1"), Value: to.StringPtr("value1")}))
	Expect(envVars).To(ContainElement(containerinstance.EnvironmentVariable{Name: to.StringPtr("key2"), Value: to.StringPtr("value2")}))
}

func (suite *ConvertTestSuite) TestConvertToAciRestartPolicyCondition() {
	Expect(toAciRestartPolicy("none")).To(Equal(containerinstance.Never))
	Expect(toAciRestartPolicy("always")).To(Equal(containerinstance.Always))
	Expect(toAciRestartPolicy("on-failure")).To(Equal(containerinstance.OnFailure))
	Expect(toAciRestartPolicy("on-failure:5")).To(Equal(containerinstance.Always))
}

func (suite *ConvertTestSuite) TestConvertToDockerRestartPolicyCondition() {
	Expect(toContainerRestartPolicy(containerinstance.Never)).To(Equal("none"))
	Expect(toContainerRestartPolicy(containerinstance.Always)).To(Equal("any"))
	Expect(toContainerRestartPolicy(containerinstance.OnFailure)).To(Equal("on-failure"))
	Expect(toContainerRestartPolicy("")).To(Equal("any"))
}

func TestConvertTestSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(ConvertTestSuite))
}
