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

	autoscaling "github.com/awslabs/goformation/v4/cloudformation/applicationautoscaling"
	"gotest.tools/v3/assert"
)

func TestAutoScaling(t *testing.T) {
	template := convertYaml(t, `
services:
  foo:
    image: hello_world
    deploy:
      x-aws-autoscaling: 
        cpu: 75
        max: 10
`, useDefaultVPC)
	target := template.Resources["FooScalableTarget"].(*autoscaling.ScalableTarget)
	assert.Check(t, target != nil)            //nolint:staticcheck
	assert.Check(t, target.MaxCapacity == 10) //nolint:staticcheck

	policy := template.Resources["FooScalingPolicy"].(*autoscaling.ScalingPolicy)
	assert.Check(t, policy != nil)                                                              //nolint:staticcheck
	assert.Check(t, policy.TargetTrackingScalingPolicyConfiguration.TargetValue == float64(75)) //nolint:staticcheck
}
