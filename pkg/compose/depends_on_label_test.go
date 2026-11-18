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

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestDependsOnLabelRoundTrip(t *testing.T) {
	dependsOn := types.DependsOnConfig{
		"db":    {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
		"cache": {Condition: types.ServiceConditionStarted, Restart: false, Required: true},
	}
	assert.DeepEqual(t, decodeDependsOnLabel(encodeDependsOnLabel(dependsOn)), dependsOn)
}

func TestDecodeDependsOnLabelBackwardCompatibility(t *testing.T) {
	assert.DeepEqual(t, decodeDependsOnLabel("db"), types.DependsOnConfig{
		"db": {Condition: ServiceConditionRunningOrHealthy, Restart: true, Required: true},
	})
	assert.DeepEqual(t, decodeDependsOnLabel("db:service_healthy"), types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
	})
}
