package amazon

import (
	"fmt"
	"strings"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/convert"
)

func (c client) Convert(project *compose.Project, loadBalancerArn *string) (*cloudformation.Template, error) {
	template := cloudformation.NewTemplate()

	vpc, err := c.GetDefaultVPC()
	if err != nil {
		return nil, err
	}

	subnets, err := c.GetSubNets(vpc)
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
		VpcId:                *vpc,
	}

	for _, service := range project.Services {
		definition, err := convert.Convert(project, service)
		if err != nil {
			return nil, err
		}

		role, err := c.GetEcsTaskExecutionRole(service)
		if err != nil {
			return nil, err
		}
		definition.TaskRoleArn = *role

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
