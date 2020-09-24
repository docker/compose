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
	"context"
	"encoding/base64"
	"fmt"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/autoscaling"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) createCapacityProvider(ctx context.Context, project *types.Project, template *cloudformation.Template) error {
	var ec2 bool
	for _, s := range project.Services {
		if requireEC2(s) {
			ec2 = true
			break
		}
	}

	if !ec2 {
		return nil
	}

	ami, err := b.SDK.GetParameter(ctx, "/aws/service/ecs/optimized-ami/amazon-linux-2/gpu/recommended")
	if err != nil {
		return err
	}

	machineType, err := guessMachineType(project)
	if err != nil {
		return err
	}

	template.Resources["CapacityProvider"] = &ecs.CapacityProvider{
		AutoScalingGroupProvider: &ecs.CapacityProvider_AutoScalingGroupProvider{
			AutoScalingGroupArn: cloudformation.Ref("AutoscalingGroup"),
			ManagedScaling: &ecs.CapacityProvider_ManagedScaling{
				TargetCapacity: 100,
			},
		},
		Tags: projectTags(project),
	}

	template.Resources["AutoscalingGroup"] = &autoscaling.AutoScalingGroup{
		LaunchConfigurationName: cloudformation.Ref("LaunchConfiguration"),
		MaxSize:                 "10", //TODO
		MinSize:                 "1",
		VPCZoneIdentifier:       b.resources.subnets,
	}

	userData := base64.StdEncoding.EncodeToString([]byte(
		fmt.Sprintf("#!/bin/bash\necho ECS_CLUSTER=%s >> /etc/ecs/ecs.config", project.Name)))

	template.Resources["LaunchConfiguration"] = &autoscaling.LaunchConfiguration{
		ImageId:            ami,
		InstanceType:       machineType,
		SecurityGroups:     b.resources.allSecurityGroups(),
		IamInstanceProfile: cloudformation.Ref("EC2InstanceProfile"),
		UserData:           userData,
	}

	template.Resources["EC2InstanceProfile"] = &iam.InstanceProfile{
		Roles: []string{cloudformation.Ref("EC2InstanceRole")},
	}

	template.Resources["EC2InstanceRole"] = &iam.Role{
		AssumeRolePolicyDocument: ec2InstanceAssumeRolePolicyDocument,
		ManagedPolicyArns: []string{
			ecsEC2InstanceRole,
		},
		Tags: projectTags(project),
	}

	cluster := template.Resources["Cluster"].(*ecs.Cluster)
	cluster.CapacityProviders = []string{
		cloudformation.Ref("CapacityProvider"),
	}

	return nil
}
