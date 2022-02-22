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

package compose

import (
	"fmt"
	"testing"

	"github.com/docker/compose/v2/pkg/api"

	moby "github.com/docker/docker/api/types"
	"gotest.tools/v3/assert"
)

func TestContainersToStacks(t *testing.T) {
	containers := []moby.Container{
		{
			ID:     "service1",
			State:  "running",
			Labels: map[string]string{api.ProjectLabel: "project1", api.ConfigFilesLabel: "/home/docker-compose.yaml"},
		},
		{
			ID:     "service2",
			State:  "running",
			Labels: map[string]string{api.ProjectLabel: "project1", api.ConfigFilesLabel: "/home/docker-compose.yaml"},
		},
		{
			ID:     "service3",
			State:  "running",
			Labels: map[string]string{api.ProjectLabel: "project2", api.ConfigFilesLabel: "/home/project2-docker-compose.yaml"},
		},
	}
	stacks, err := containersToStacks(containers)
	assert.NilError(t, err)
	assert.DeepEqual(t, stacks, []api.Stack{
		{
			ID:          "project1",
			Name:        "project1",
			Status:      "running(2)",
			ConfigFiles: "/home/docker-compose.yaml",
		},
		{
			ID:          "project2",
			Name:        "project2",
			Status:      "running(1)",
			ConfigFiles: "/home/project2-docker-compose.yaml",
		},
	})
}

func TestStacksMixedStatus(t *testing.T) {
	assert.Equal(t, combinedStatus([]string{"running"}), "running(1)")
	assert.Equal(t, combinedStatus([]string{"running", "running", "running"}), "running(3)")
	assert.Equal(t, combinedStatus([]string{"running", "exited", "running"}), "exited(1), running(2)")
}

func TestCombinedConfigFiles(t *testing.T) {
	containersByLabel := map[string][]moby.Container{
		"project1": {
			{
				ID:     "service1",
				State:  "running",
				Labels: map[string]string{api.ProjectLabel: "project1", api.ConfigFilesLabel: "/home/docker-compose.yaml"},
			},
			{
				ID:     "service2",
				State:  "running",
				Labels: map[string]string{api.ProjectLabel: "project1", api.ConfigFilesLabel: "/home/docker-compose.yaml"},
			},
		},
		"project2": {
			{
				ID:     "service3",
				State:  "running",
				Labels: map[string]string{api.ProjectLabel: "project2", api.ConfigFilesLabel: "/home/project2-docker-compose.yaml"},
			},
		},
		"project3": {
			{
				ID:     "service4",
				State:  "running",
				Labels: map[string]string{api.ProjectLabel: "project3"},
			},
		},
	}

	testData := map[string]struct {
		ConfigFiles string
		Error       error
	}{
		"project1": {ConfigFiles: "/home/docker-compose.yaml", Error: nil},
		"project2": {ConfigFiles: "/home/project2-docker-compose.yaml", Error: nil},
		"project3": {ConfigFiles: "", Error: fmt.Errorf("No label %q set on container %q of compose project", api.ConfigFilesLabel, "service4")},
	}

	for project, containers := range containersByLabel {
		configFiles, err := combinedConfigFiles(containers)

		expected := testData[project]

		if expected.Error != nil {
			assert.Equal(t, err.Error(), expected.Error.Error())
		} else {
			assert.Equal(t, err, expected.Error)
		}
		assert.Equal(t, configFiles, expected.ConfigFiles)
	}
}
