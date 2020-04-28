package amazon

import (
	"context"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/convert"
)

func (c client) Convert(ctx context.Context, project *compose.Project) (*cloudformation.Template, error) {
	template := cloudformation.NewTemplate()
	vpc, err := c.api.GetDefaultVPC(ctx)
	if err != nil {
		return nil, err
	}

	subnets, err := c.api.GetSubNets(ctx, vpc)
	if err != nil {
		return nil, err
	}

	var ingresses = []ec2.SecurityGroup_Ingress{}
	for _, service := range project.Services {
		for _, port := range service.Ports {
			ingresses = append(ingresses, ec2.SecurityGroup_Ingress{
				CidrIp:      "0.0.0.0/0",
				Description: fmt.Sprintf("%s:%d/%s", service.Name, port.Target, port.Protocol),
				FromPort:    int(port.Target),
				IpProtocol:  strings.ToUpper(port.Protocol),
				ToPort:      int(port.Target),
			})
		}
	}

	securityGroup := fmt.Sprintf("%s Security Group", project.Name)
	template.Resources["SecurityGroup"] = &ec2.SecurityGroup{
		GroupDescription:     securityGroup,
		GroupName:            securityGroup,
		SecurityGroupIngress: ingresses,
		VpcId:                vpc,
	}

	for _, service := range project.Services {
		definition, err := convert.Convert(project, service)
		if err != nil {
			return nil, err
		}

		role, err := c.GetEcsTaskExecutionRole(ctx, service)
		if err != nil {
			return nil, err
		}
		definition.TaskRoleArn = role

		taskDefinition := fmt.Sprintf("%sTaskDefinition", service.Name)
		template.Resources[taskDefinition] = definition

		template.Resources[service.Name] = &ecs.Service{
			Cluster:      c.Cluster,
			DesiredCount: 1,
			LaunchType:   ecsapi.LaunchTypeFargate,
			NetworkConfiguration: &ecs.Service_NetworkConfiguration{
				AwsvpcConfiguration: &ecs.Service_AwsVpcConfiguration{
					AssignPublicIp: ecsapi.AssignPublicIpEnabled,
					SecurityGroups: []string{cloudformation.Ref("SecurityGroup")},
					Subnets:        subnets,
				},
			},
			SchedulingStrategy: ecsapi.SchedulingStrategyReplica,
			ServiceName:        service.Name,
			TaskDefinition:     cloudformation.Ref(taskDefinition),
		}
	}
	return template, nil
}

const ECSTaskExecutionPolicy = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"

var defaultTaskExecutionRole string

// GetEcsTaskExecutionRole retrieve the role ARN to apply for task execution
func (c client) GetEcsTaskExecutionRole(ctx context.Context, spec types.ServiceConfig) (string, error) {
	if arn, ok := spec.Extras["x-ecs-TaskExecutionRole"]; ok {
		return arn.(string), nil
	}
	if defaultTaskExecutionRole != "" {
		return defaultTaskExecutionRole, nil
	}

	logrus.Debug("Retrieve Task Execution Role")
	entities, err := c.api.ListRolesForPolicy(ctx, ECSTaskExecutionPolicy)
	if err != nil {
		return "", err
	}
	if len(entities) == 0 {
		return "", fmt.Errorf("no Role is attached to AmazonECSTaskExecutionRole Policy, please provide an explicit task execution role")
	}
	if len(entities) > 1 {
		return "", fmt.Errorf("multiple Roles are attached to AmazonECSTaskExecutionRole Policy, please provide an explicit task execution role")
	}

	arn, err := c.api.GetRoleArn(ctx, entities[0])
	if err != nil {
		return "", err
	}
	defaultTaskExecutionRole = arn
	return arn, nil
}

type convertAPI interface {
	GetDefaultVPC(ctx context.Context) (string, error)
	GetSubNets(ctx context.Context, vpcID string) ([]string, error)
	ListRolesForPolicy(ctx context.Context, policy string) ([]string, error)
	GetRoleArn(ctx context.Context, name string) (string, error)
}
