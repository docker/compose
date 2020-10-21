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
	"encoding/json"
	"fmt"

	applicationautoscaling2 "github.com/aws/aws-sdk-go/service/applicationautoscaling"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/applicationautoscaling"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/compose-spec/compose-go/types"
)

type autoscalingConfig struct {
	Memory int `json:"memory,omitempty"`
	CPU    int `json:"cpu,omitempty"`
	Min    int `json:"min,omitempty"`
	Max    int `json:"max,omitempty"`
}

func (b *ecsAPIService) createAutoscalingPolicy(project *types.Project, resources awsResources, template *cloudformation.Template, service types.ServiceConfig) error {
	if service.Deploy == nil {
		return nil
	}
	v, ok := service.Deploy.Extensions[extensionAutoScaling]
	if !ok {
		return nil
	}

	marshalled, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var config autoscalingConfig
	err = json.Unmarshal(marshalled, &config)
	if err != nil {
		return err
	}

	if config.Memory != 0 && config.CPU != 0 {
		return fmt.Errorf("%s can't be set with both cpu and memory targets", extensionAutoScaling)
	}
	if config.Max == 0 {
		return fmt.Errorf("%s MUST define max replicas", extensionAutoScaling)
	}

	role := fmt.Sprintf("%sAutoScalingRole", normalizeResourceName(service.Name))
	template.Resources[role] = &iam.Role{
		AssumeRolePolicyDocument: ausocalingAssumeRolePolicyDocument,
		Path:                     "/",
		Policies: []iam.Role_Policy{
			{
				PolicyDocument: &PolicyDocument{
					Statement: []PolicyStatement{
						{
							Effect: "Allow",
							Action: []string{
								actionAutoScaling,
								actionDescribeService,
								actionUpdateService,
								actionGetMetrics,
							},
							Resource: []string{cloudformation.Ref(serviceResourceName(service.Name))},
						},
					},
				},
				PolicyName: "service-autoscaling",
			},
		},
		Tags: serviceTags(project, service),
	}

	// Why isn't this just the service ARN ?????
	resourceID := cloudformation.Join("/", []string{"service", resources.cluster.ID(), cloudformation.GetAtt(serviceResourceName(service.Name), "Name")})

	target := fmt.Sprintf("%sScalableTarget", normalizeResourceName(service.Name))
	template.Resources[target] = &applicationautoscaling.ScalableTarget{
		MaxCapacity:                config.Max,
		MinCapacity:                config.Min,
		ResourceId:                 resourceID,
		RoleARN:                    cloudformation.GetAtt(role, "Arn"),
		ScalableDimension:          applicationautoscaling2.ScalableDimensionEcsServiceDesiredCount,
		ServiceNamespace:           applicationautoscaling2.ServiceNamespaceEcs,
		AWSCloudFormationDependsOn: []string{serviceResourceName(service.Name)},
	}

	var (
		metric        = applicationautoscaling2.MetricTypeEcsserviceAverageCpuutilization
		targetPercent = config.CPU
	)
	if config.Memory != 0 {
		metric = applicationautoscaling2.MetricTypeEcsserviceAverageMemoryUtilization
		targetPercent = config.Memory
	}

	policy := fmt.Sprintf("%sScalingPolicy", normalizeResourceName(service.Name))
	template.Resources[policy] = &applicationautoscaling.ScalingPolicy{
		PolicyType:                     "TargetTrackingScaling",
		PolicyName:                     policy,
		ScalingTargetId:                cloudformation.Ref(target),
		StepScalingPolicyConfiguration: nil,
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.ScalingPolicy_TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &applicationautoscaling.ScalingPolicy_PredefinedMetricSpecification{
				PredefinedMetricType: metric,
			},
			ScaleOutCooldown: 60,
			ScaleInCooldown:  60,
			TargetValue:      float64(targetPercent),
		},
	}
	return nil
}
