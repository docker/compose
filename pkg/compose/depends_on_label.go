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
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

// encodeDependsOnLabel writes the depends_on of a service the way containers
// record it in api.DependenciesLabel: a comma-separated list of
// service:condition:restart entries.
func encodeDependsOnLabel(dependsOn types.DependsOnConfig) string {
	var dependencies []string
	for dep, d := range dependsOn {
		dependencies = append(dependencies, fmt.Sprintf("%s:%s:%t", dep, d.Condition, d.Restart))
	}
	return strings.Join(dependencies, ",")
}

// decodeDependsOnLabel reads back the depends_on recorded by encodeDependsOnLabel.
// Labels written by older versions may lack the condition and the restart flag.
func decodeDependsOnLabel(label string) types.DependsOnConfig {
	dependsOn := types.DependsOnConfig{}
	for dc := range strings.SplitSeq(label, ",") {
		dcArr := strings.Split(dc, ":")
		condition := ServiceConditionRunningOrHealthy
		// Let's restart the dependency by default if we don't have the info stored in the label
		restart := true
		required := true
		dependency := dcArr[0]

		// backward compatibility
		if len(dcArr) > 1 {
			condition = dcArr[1]
			if len(dcArr) > 2 {
				restart, _ = strconv.ParseBool(dcArr[2])
			}
		}
		dependsOn[dependency] = types.ServiceDependency{Condition: condition, Restart: restart, Required: required}
	}
	return dependsOn
}
