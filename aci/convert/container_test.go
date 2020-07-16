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
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"

	"github.com/docker/api/containers"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
)

type ContainerConvertTestSuite struct {
	suite.Suite
}

func (suite *ContainerConvertTestSuite) TestConvertContainerEnvironment() {
	container := containers.ContainerConfig{
		ID:          "container1",
		Environment: []string{"key1=value1", "key2", "key3=value3"},
	}
	project, err := ContainerToComposeProject(container)
	Expect(err).To(BeNil())
	service1 := project.Services[0]
	Expect(service1.Name).To(Equal(container.ID))
	Expect(service1.Environment).To(Equal(types.MappingWithEquals{
		"key1": to.StringPtr("value1"),
		"key2": nil,
		"key3": to.StringPtr("value3"),
	}))
}

func (suite *ContainerConvertTestSuite) TestConvertRestartPolicy() {
	container := containers.ContainerConfig{
		ID:                     "container1",
		RestartPolicyCondition: "no",
	}
	project, err := ContainerToComposeProject(container)
	Expect(err).To(BeNil())
	service1 := project.Services[0]
	Expect(service1.Name).To(Equal(container.ID))
	Expect(service1.Deploy.RestartPolicy.Condition).To(Equal("none"))
}

func TestContainerConvertTestSuite(t *testing.T) {
	RegisterTestingT(t)
	suite.Run(t, new(ContainerConvertTestSuite))
}
