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
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
)

// warnIgnoredDeployAttributes emits a warning for deploy attributes that are
// ignored by Compose when running in standalone (non-Swarm) mode, so users
// don't silently rely on configuration that has no effect.
// See https://github.com/docker/compose/issues/13150
func warnIgnoredDeployAttributes(project *types.Project) {
	for _, svc := range project.Services {
		ignored := collectIgnoredDeployAttributes(svc.Deploy)
		if len(ignored) == 0 {
			continue
		}
		sort.Strings(ignored)
		logrus.Warnf(
			"Service %q uses deploy attributes that are ignored by Compose in standalone mode: %s",
			svc.Name,
			strings.Join(ignored, ", "),
		)
	}
}

// collectIgnoredDeployAttributes returns the deploy sub-attributes that are
// consistently ignored in standalone mode. Attributes that are still honored
// (replicas, device reservations, restart_policy fallback) are intentionally
// excluded to avoid noisy or misleading warnings.
func collectIgnoredDeployAttributes(deploy *types.DeployConfig) []string {
	if deploy == nil {
		return nil
	}
	var ignored []string
	if len(deploy.Placement.Constraints) > 0 {
		ignored = append(ignored, "placement.constraints")
	}
	if len(deploy.Placement.Preferences) > 0 {
		ignored = append(ignored, "placement.preferences")
	}
	if deploy.UpdateConfig != nil {
		ignored = append(ignored, "update_config")
	}
	if deploy.RollbackConfig != nil {
		ignored = append(ignored, "rollback_config")
	}
	if deploy.EndpointMode != "" {
		ignored = append(ignored, "endpoint_mode")
	}
	if deploy.Mode != "" {
		ignored = append(ignored, "mode")
	}
	return ignored
}
