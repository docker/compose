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

package ecs

import (
	"testing"

	"github.com/awslabs/goformation/v4/cloudformation/autoscaling"
	"gotest.tools/v3/assert"
)

func TestUserDefinedAMI(t *testing.T) {
	template := convertYaml(t, `
services:
  test:
    image: "image"
    deploy:
      placement:
        constraints:
          - "node.ami == ami123456789"
          - "node.machine == t0.femto"
      resources:
        # devices:
        #   - capabilities: ["gpu"]
        reservations:
          memory: 8Gb
          generic_resources:
            - discrete_resource_spec:
                kind: gpus
                value: 1                    
`, useDefaultVPC)
	lc := template.Resources["LaunchConfiguration"].(*autoscaling.LaunchConfiguration)
	assert.Check(t, lc.ImageId == "ami123456789")
	assert.Check(t, lc.InstanceType == "t0.femto")
}
