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
			Labels: map[string]string{api.ProjectLabel: "project1"},
		},
		{
			ID:     "service2",
			State:  "running",
			Labels: map[string]string{api.ProjectLabel: "project1"},
		},
		{
			ID:     "service3",
			State:  "running",
			Labels: map[string]string{api.ProjectLabel: "project2"},
		},
	}
	stacks, err := containersToStacks(containers)
	assert.NilError(t, err)
	assert.DeepEqual(t, stacks, []api.Stack{
		{
			ID:     "project1",
			Name:   "project1",
			Status: "running(2)",
		},
		{
			ID:     "project2",
			Name:   "project2",
			Status: "running(1)",
		},
	})
}

func TestStacksMixedStatus(t *testing.T) {
	assert.Equal(t, combinedStatus([]string{"running"}), "running(1)")
	assert.Equal(t, combinedStatus([]string{"running", "running", "running"}), "running(3)")
	assert.Equal(t, combinedStatus([]string{"running", "exited", "running"}), "exited(1), running(2)")
}
