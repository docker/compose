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
	"fmt"

	applicationautoscaling2 "github.com/aws/aws-sdk-go/service/applicationautoscaling"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/applicationautoscaling"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) createAutoscalingPolicy(project *types.Project, resources awsResources, template *cloudformation.Template, service types.ServiceConfig) {
	if service.Deploy == nil {
		return
	}
	v, ok := service.Deploy.Extensions[extensionAutoScaling]
	if !ok {
		return
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
	resourceID := cloudformation.Join("/", []string{"service", resources.cluster, cloudformation.GetAtt(serviceResourceName(service.Name), "Name")})

	target := fmt.Sprintf("%sScalableTarget", normalizeResourceName(service.Name))
	template.Resources[target] = &applicationautoscaling.ScalableTarget{
		MaxCapacity:                10,
		MinCapacity:                0,
		ResourceId:                 resourceID,
		RoleARN:                    cloudformation.GetAtt(role, "Arn"),
		ScalableDimension:          applicationautoscaling2.ScalableDimensionEcsServiceDesiredCount,
		ServiceNamespace:           applicationautoscaling2.ServiceNamespaceEcs,
		AWSCloudFormationDependsOn: []string{serviceResourceName(service.Name)},
	}

	policy := fmt.Sprintf("%sScalingPolicy", normalizeResourceName(service.Name))
	template.Resources[policy] = &applicationautoscaling.ScalingPolicy{
		PolicyType:                     "TargetTrackingScaling",
		PolicyName:                     policy,
		ScalingTargetId:                cloudformation.Ref(target),
		StepScalingPolicyConfiguration: nil,
		TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.ScalingPolicy_TargetTrackingScalingPolicyConfiguration{
			PredefinedMetricSpecification: &applicationautoscaling.ScalingPolicy_PredefinedMetricSpecification{
				PredefinedMetricType: applicationautoscaling2.MetricTypeEcsserviceAverageCpuutilization,
			},
			ScaleOutCooldown: 60,
			ScaleInCooldown:  60,
			TargetValue:      float64(v.(int)),
		},
	}
}
