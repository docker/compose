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

	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/ecs"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/secrets"
)

const (
	awsTypeCapacityProvider = "AWS::ECS::CapacityProvider"
	awsTypeAutoscalingGroup = "AWS::AutoScaling::AutoScalingGroup"
)

//go:generate mockgen -destination=./aws_mock.go -self_package "github.com/docker/compose-cli/ecs" -package=ecs . API

// API hides aws-go-sdk into a simpler, focussed API subset
type API interface {
	CheckRequirements(ctx context.Context, region string) error
	ResolveCluster(ctx context.Context, nameOrArn string) (awsResource, error)
	CreateCluster(ctx context.Context, name string) (string, error)
	CheckVPC(ctx context.Context, vpcID string) error
	GetDefaultVPC(ctx context.Context) (string, error)
	GetSubNets(ctx context.Context, vpcID string) ([]awsResource, error)
	IsPublicSubnet(ctx context.Context, subNetID string) (bool, error)
	GetRoleArn(ctx context.Context, name string) (string, error)
	StackExists(ctx context.Context, name string) (bool, error)
	CreateStack(ctx context.Context, name string, region string, template []byte) error
	CreateChangeSet(ctx context.Context, name string, region string, template []byte) (string, error)
	UpdateStack(ctx context.Context, changeset string) error
	WaitStackComplete(ctx context.Context, name string, operation int) error
	GetStackID(ctx context.Context, name string) (string, error)
	ListStacks(ctx context.Context, name string) ([]compose.Stack, error)
	GetStackClusterID(ctx context.Context, stack string) (string, error)
	GetServiceTaskDefinition(ctx context.Context, cluster string, serviceArns []string) (map[string]string, error)
	ListStackServices(ctx context.Context, stack string) ([]string, error)
	GetServiceTasks(ctx context.Context, cluster string, service string, stopped bool) ([]*ecs.Task, error)
	GetTaskStoppedReason(ctx context.Context, cluster string, taskArn string) (string, error)
	DescribeStackEvents(ctx context.Context, stackID string) ([]*cloudformation.StackEvent, error)
	ListStackParameters(ctx context.Context, name string) (map[string]string, error)
	ListStackResources(ctx context.Context, name string) (stackResources, error)
	DeleteStack(ctx context.Context, name string) error
	CreateSecret(ctx context.Context, secret secrets.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (secrets.Secret, error)
	ListSecrets(ctx context.Context) ([]secrets.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
	GetLogs(ctx context.Context, name string, consumer func(service, container, message string)) error
	DescribeService(ctx context.Context, cluster string, arn string) (compose.ServiceStatus, error)
	DescribeServiceTasks(ctx context.Context, cluster string, project string, service string) ([]compose.ContainerSummary, error)
	getURLWithPortMapping(ctx context.Context, targetGroupArns []string) ([]compose.PortPublisher, error)
	ListTasks(ctx context.Context, cluster string, family string) ([]string, error)
	GetPublicIPs(ctx context.Context, interfaces ...string) (map[string]string, error)
	ResolveLoadBalancer(ctx context.Context, nameOrArn string) (awsResource, string, string, []awsResource, error)
	GetLoadBalancerURL(ctx context.Context, arn string) (string, error)
	GetParameter(ctx context.Context, name string) (string, error)
	SecurityGroupExists(ctx context.Context, sg string) (bool, error)
	DeleteCapacityProvider(ctx context.Context, arn string) error
	DeleteAutoscalingGroup(ctx context.Context, arn string) error
	ResolveFileSystem(ctx context.Context, id string) (awsResource, error)
	ListFileSystems(ctx context.Context, tags map[string]string) ([]awsResource, error)
	CreateFileSystem(ctx context.Context, tags map[string]string, options VolumeCreateOptions) (awsResource, error)
	DeleteFileSystem(ctx context.Context, id string) error
}
