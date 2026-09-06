/*
   Copyright 2026 Docker Compose CLI authors

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

func TestCollectIgnoredDeployAttributes(t *testing.T) {
	updateConfig := &types.UpdateConfig{}
	t.Run("nil deploy", func(t *testing.T) {
		assert.DeepEqual(t, collectIgnoredDeployAttributes(nil), []string(nil))
	})
	t.Run("no deploy attributes", func(t *testing.T) {
		assert.DeepEqual(t, collectIgnoredDeployAttributes(&types.DeployConfig{}), []string(nil))
	})
	t.Run("supported attributes only", func(t *testing.T) {
		replicas := 2
		deploy := &types.DeployConfig{
			Replicas: &replicas,
		}
		assert.DeepEqual(t, collectIgnoredDeployAttributes(deploy), []string(nil))
	})
	t.Run("ignored attributes", func(t *testing.T) {
		deploy := &types.DeployConfig{
			Mode:           "replicated",
			EndpointMode:   "vip",
			UpdateConfig:   updateConfig,
			RollbackConfig: updateConfig,
			Placement: types.Placement{
				Constraints: []string{"node.role==manager"},
				Preferences: []types.PlacementPreferences{{Spread: "node.labels.zone"}},
			},
		}
		assert.DeepEqual(t, collectIgnoredDeployAttributes(deploy), []string{
			"placement.constraints",
			"placement.preferences",
			"update_config",
			"rollback_config",
			"endpoint_mode",
			"mode",
		})
	})
}
