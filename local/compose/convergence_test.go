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

	"github.com/compose-spec/compose-go/types"
	"gotest.tools/assert"
)

func TestContainerName(t *testing.T) {
	var replicas uint64 = 1
	s := types.ServiceConfig{
		Name:          "testservicename",
		ContainerName: "testcontainername",
		Scale:         1,
		Deploy:        &types.DeployConfig{},
	}
	ret, err := getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, s.Scale)

	s.Scale = 0
	s.Deploy.Replicas = &replicas
	ret, err = getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, int(*s.Deploy.Replicas))

	s.Deploy.Replicas = nil
	s.Scale = 2
	_, err = getScale(s)
	assert.Error(t, err, fmt.Sprintf(doubledContainerNameWarning, s.Name, s.ContainerName))

	replicas = 2
	s.Deploy.Replicas = &replicas
	s.Scale = 0
	_, err = getScale(s)
	assert.Error(t, err, fmt.Sprintf(doubledContainerNameWarning, s.Name, s.ContainerName))
}
