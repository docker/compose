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

	"github.com/awslabs/goformation/v4/cloudformation"
)

const (
	ecsTaskExecutionPolicy = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
	ecrReadOnlyPolicy      = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
	ecsEC2InstanceRole     = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"

	actionGetSecretValue  = "secretsmanager:GetSecretValue"
	actionGetParameters   = "ssm:GetParameters"
	actionDecrypt         = "kms:Decrypt"
	actionAutoScaling     = "application-autoscaling:*"
	actionGetMetrics      = "cloudwatch:GetMetricStatistics"
	actionDescribeService = "ecs:DescribeServices"
	actionUpdateService   = "ecs:UpdateService"
)

var (
	ecsTaskAssumeRolePolicyDocument     = policyDocument("ecs-tasks.amazonaws.com")
	ec2InstanceAssumeRolePolicyDocument = policyDocument("ec2.amazonaws.com")
	ausocalingAssumeRolePolicyDocument  = policyDocument("application-autoscaling.amazonaws.com")
)

func policyDocument(service string) PolicyDocument {
	return PolicyDocument{
		Version: "2012-10-17", // https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_version.html
		Statement: []PolicyStatement{
			{
				Effect: "Allow",
				Principal: PolicyPrincipal{
					Service: service,
				},
				Action: []string{"sts:AssumeRole"},
			},
		},
	}
}

func volumeMountPolicyDocument(volume string, filesystem string) PolicyDocument {
	ap := fmt.Sprintf("%sAccessPoint", normalizeResourceName(volume))
	return PolicyDocument{
		Version: "2012-10-17", // https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_version.html
		Statement: []PolicyStatement{
			{
				Effect: "Allow",
				Resource: []string{
					filesystem,
				},
				Action: []string{
					"elasticfilesystem:ClientMount",
					"elasticfilesystem:ClientWrite",
					"elasticfilesystem:ClientRootAccess",
				},
				Condition: Condition{
					StringEquals: map[string]string{
						"elasticfilesystem:AccessPointArn": cloudformation.Ref(ap),
					},
				},
			},
		},
	}
}

// PolicyDocument describes an IAM policy document
// could alternatively depend on https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/cmd/clusterawsadm/api/iam/v1alpha1/types.go
type PolicyDocument struct {
	Version   string            `json:",omitempty"`
	Statement []PolicyStatement `json:",omitempty"`
}

// PolicyStatement describes an IAM policy statement
type PolicyStatement struct {
	Effect    string          `json:",omitempty"`
	Action    []string        `json:",omitempty"`
	Principal PolicyPrincipal `json:",omitempty"`
	Resource  []string        `json:",omitempty"`
	Condition Condition       `json:",omitempty"`
}

// PolicyPrincipal describes an IAM policy principal
type PolicyPrincipal struct {
	Service string `json:",omitempty"`
}

// Condition is the map of all conditions in the statement entry.
type Condition struct {
	StringEquals map[string]string `json:",omitempty"`
	Bool         map[string]string `json:",omitempty"`
}
