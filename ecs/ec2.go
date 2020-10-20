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
	"strings"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/autoscaling"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/compose-spec/compose-go/types"
)

const (
	placementConstraintAMI     = "node.ami == "
	placementConstraintMachine = "node.machine == "
)

func (b *ecsAPIService) createCapacityProvider(ctx context.Context, project *types.Project, template *cloudformation.Template, resources awsResources) error {
	var (
		ec2         bool
		ami         string
		machineType string
	)
	for _, service := range project.Services {
		if requireEC2(service) {
			ec2 = true
			// TODO once we can assign a service to a CapacityProvider, we could run this _per service_
			ami, machineType = getUserDefinedMachine(service)
			break
		}
	}

	if !ec2 {
		return nil
	}

	if ami == "" {
		recommended, err := b.aws.GetParameter(ctx, "/aws/service/ecs/optimized-ami/amazon-linux-2/gpu/recommended")
		if err != nil {
			return err
		}
		ami = recommended
	}

	if machineType == "" {
		t, err := guessMachineType(project)
		if err != nil {
			return err
		}
		machineType = t
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
		VPCZoneIdentifier:       resources.subnetsIDs(),
	}

	userData := base64.StdEncoding.EncodeToString([]byte(
		fmt.Sprintf("#!/bin/bash\necho ECS_CLUSTER=%s >> /etc/ecs/ecs.config", project.Name)))

	template.Resources["LaunchConfiguration"] = &autoscaling.LaunchConfiguration{
		ImageId:            ami,
		InstanceType:       machineType,
		SecurityGroups:     resources.allSecurityGroups(),
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

func getUserDefinedMachine(s types.ServiceConfig) (ami string, machineType string) {
	if s.Deploy != nil {
		for _, s := range s.Deploy.Placement.Constraints {
			if strings.HasPrefix(s, placementConstraintAMI) {
				ami = s[len(placementConstraintAMI):]
			}
			if strings.HasPrefix(s, placementConstraintMachine) {
				machineType = s[len(placementConstraintMachine):]
			}
		}
	}
	return ami, machineType
}
