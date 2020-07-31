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

package aci

import (
	"context"
	"testing"

	"github.com/docker/api/containers"

	"gotest.tools/v3/assert"
)

func TestGetContainerName(t *testing.T) {
	group, container := getGroupAndContainerName("docker1234")
	assert.Equal(t, group, "docker1234")
	assert.Equal(t, container, "docker1234")

	group, container = getGroupAndContainerName("compose_service1")
	assert.Equal(t, group, "compose")
	assert.Equal(t, container, "service1")

	group, container = getGroupAndContainerName("compose_stack_service1")
	assert.Equal(t, group, "compose_stack")
	assert.Equal(t, container, "service1")
}

func TestErrorMessageDeletingContainerFromComposeApplication(t *testing.T) {
	service := aciContainerService{}
	err := service.Delete(context.TODO(), "compose-app_service1", false)
	assert.Error(t, err, "cannot delete service \"service1\" from compose application \"compose-app\", you can delete the entire compose app with docker compose down --project-name compose-app")
}

func TestErrorMessageRunSingleContainerNameWithComposeSeparator(t *testing.T) {
	service := aciContainerService{}
	err := service.Run(context.TODO(), containers.ContainerConfig{ID: "container_name"})
	assert.Error(t, err, "invalid container name. ACI container name cannot include \"_\"")
}

func TestVerifyCommand(t *testing.T) {
	err := verifyExecCommand("command") // Command without an argument
	assert.NilError(t, err)
	err = verifyExecCommand("command argument") // Command with argument
	assert.Error(t, err, "ACI exec command does not accept arguments to the command. "+
		"Only the binary should be specified")
}
