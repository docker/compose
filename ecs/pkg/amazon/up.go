package amazon

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/convert"
	"github.com/sirupsen/logrus"
)

func (c *client) ComposeUp(project *compose.Project, loadBalancerArn *string) error {
	type mapping struct {
		service *types.ServiceConfig
		task *ecs.RegisterTaskDefinitionInput
	}
	mappings := []mapping{}
	for _, service := range project.Services {
		task, err := convert.Convert(project, service)
		if err != nil {
			return err
		}
		mappings = append(mappings, mapping{
			service: &service,
			task:    task,
		})
	}

	vpc, err := c.GetDefaultVPC()
	if err != nil {
		return err
	}
	subnets, err := c.GetSubNets(vpc)
	if err != nil {
		return err
	}

	securityGroup, err := c.CreateSecurityGroup(project, vpc)
	if err != nil {
		return err
	}

	if loadBalancerArn == nil {
		loadBalancerArn, err = c.CreateLoadBalancer(project, subnets)
		if err != nil {
			return err
		}
	}

	logGroup, err := c.GetOrCreateLogGroup(project)
	if err != nil {
		return err
	}

	for _, mapping := range mappings {
		ingress := []*ecs.LoadBalancer{}
		for _, port := range mapping.service.Ports {
			name := fmt.Sprintf("%s-%s-%d-%s", project.Name, mapping.service.Name, port.Target, port.Protocol)
			targetgroup, err := c.CreateTargetGroup(name, vpc, port)
			if err != nil {
				return err
			}
			ingress = append(ingress, &ecs.LoadBalancer{
				ContainerName:  aws.String(mapping.service.Name),
				ContainerPort:  aws.Int64(int64(port.Target)),
				TargetGroupArn: targetgroup,
			})

			err = c.CreateListener(port, loadBalancerArn, targetgroup)
			if err != nil {
				return err
			}
		}

		_, err = c.CreateService(project, mapping.service, mapping.task, securityGroup, subnets, logGroup, ingress)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *client) CreateService(project *compose.Project, service *types.ServiceConfig, task *ecs.RegisterTaskDefinitionInput, securityGroup *string, subnets []*string, logGroup *string, ingress []*ecs.LoadBalancer) (*string, error) {
	role, err := c.GetEcsTaskExecutionRole(service)
	if err != nil {
		return nil, err
	}

	task.ExecutionRoleArn = role

	for _, def := range task.ContainerDefinitions {
		def.LogConfiguration.Options["awslogs-group"] = logGroup
		def.LogConfiguration.Options["awslogs-stream-prefix"] = aws.String(service.Name)
		def.LogConfiguration.Options["awslogs-region"] = aws.String(c.Region)
	}

	arn, err := c.RegisterTaskDefinition(task)
	if err != nil {
		return nil, err
	}

	logrus.Debug("Create Service")
	created, err := c.ECS.CreateService(&ecs.CreateServiceInput{
		Cluster:      aws.String(c.Cluster),
		DesiredCount: aws.Int64(1), // FIXME get from deploy options
		LaunchType:   aws.String(ecs.LaunchTypeFargate), //FIXME use service.Isolation tro select EC2 vs Fargate
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				AssignPublicIp: aws.String(ecs.AssignPublicIpEnabled),
				SecurityGroups: []*string{securityGroup},
				Subnets:        subnets,
			},
		},
		ServiceName:        aws.String(service.Name),
		SchedulingStrategy: aws.String(ecs.SchedulingStrategyReplica),
		TaskDefinition:     arn,
		LoadBalancers: ingress,
	})

	for _, port := range service.Ports {
		err = c.ExposePort(securityGroup, port)
		if err != nil {
			return nil, err
		}
	}

	return created.Service.ServiceArn, err
}
