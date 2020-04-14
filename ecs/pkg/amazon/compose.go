package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeUp(project *compose.Project) error {
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

	logGroup, err := c.GetOrCreateLogGroup(project.Name)
	if err != nil {
		return err
	}

	for _, service := range project.Services {
		err = c.CreateService(service, securityGroup, subnets, logGroup)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *client) CreateService(service types.ServiceConfig, securityGroup *string, subnets []*string, logGroup *string) error {
	task, err := ConvertToTaskDefinition(service)
	if err != nil {
		return err
	}

	role, err := c.GetEcsTaskExecutionRole(service)
	if err != nil {
		return err
	}

	task.ExecutionRoleArn = role

	for _, def := range task.ContainerDefinitions {
		def.LogConfiguration.Options["awslogs-group"] = logGroup
		def.LogConfiguration.Options["awslogs-stream-prefix"] = aws.String(service.Name)
		def.LogConfiguration.Options["awslogs-region"] = aws.String(c.Region)
	}

	arn, err := c.RegisterTaskDefinition(task)
	if err != nil {
		return err
	}

	_, err = c.ECS.CreateService(&ecs.CreateServiceInput{
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
	})
	return err
}
