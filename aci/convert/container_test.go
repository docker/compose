/*
   Copyright 2020 Docker Compose CLI authors

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
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/api/containers"
)

func TestConvertContainerEnvironment(t *testing.T) {
	container := containers.ContainerConfig{
		ID:          "container1",
		Command:     []string{"echo", "Hello!"},
		Environment: []string{"key1=value1", "key2", "key3=value3"},
	}
	project, err := ContainerToComposeProject(container)
	assert.NilError(t, err)
	service1 := project.Services[0]
	assert.Equal(t, service1.Name, container.ID)
	assert.DeepEqual(t, []string(service1.Command), container.Command)
	assert.DeepEqual(t, service1.Environment, types.MappingWithEquals{
		"key1": to.StringPtr("value1"),
		"key2": nil,
		"key3": to.StringPtr("value3"),
	})
}

func TestConvertRestartPolicy(t *testing.T) {
	container := containers.ContainerConfig{
		ID:                     "container1",
		RestartPolicyCondition: "none",
	}
	project, err := ContainerToComposeProject(container)
	assert.NilError(t, err)
	service1 := project.Services[0]
	assert.Equal(t, service1.Name, container.ID)
	assert.Equal(t, service1.Deploy.RestartPolicy.Condition, "none")
}

func TestConvertDomainName(t *testing.T) {
	container := containers.ContainerConfig{
		ID:         "container1",
		DomainName: "myapp",
	}
	project, err := ContainerToComposeProject(container)
	assert.NilError(t, err)
	service1 := project.Services[0]
	assert.Equal(t, service1.Name, container.ID)
	assert.Equal(t, service1.DomainName, "myapp")
}

func TestConvertEnvVariables(t *testing.T) {
	container := containers.ContainerConfig{
		ID: "container1",
		Environment: []string{
			"key=value",
			"key2=value=with=equal",
		},
	}
	project, err := ContainerToComposeProject(container)
	assert.NilError(t, err)
	service1 := project.Services[0]
	assert.Equal(t, service1.Name, container.ID)
	assert.DeepEqual(t, service1.Environment, types.MappingWithEquals{
		"key":  to.StringPtr("value"),
		"key2": to.StringPtr("value=with=equal"),
	})
}
