package amazon

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/awslabs/goformation/v4/cloudformation/logs"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	cloudmap "github.com/awslabs/goformation/v4/cloudformation/servicediscovery"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c client) Convert(ctx context.Context, project *compose.Project) (*cloudformation.Template, error) {
	template := cloudformation.NewTemplate()
	vpc, err := c.GetVPC(ctx, project)
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

	logGroup := fmt.Sprintf("/docker-compose/%s", project.Name)
	template.Resources["LogGroup"] = &logs.LogGroup{
		LogGroupName: logGroup,
	}

	// Private DNS namespace will allow DNS name for the services to be <service>.<project>.local
	template.Resources["CloudMap"] = &cloudmap.PrivateDnsNamespace{
		Description: fmt.Sprintf("Service Map for Docker Compose project %s", project.Name),
		Name:        fmt.Sprintf("%s.local", project.Name),
		Vpc:         vpc,
	}

	for _, service := range project.Services {
		definition, err := Convert(project, service)
		if err != nil {
			return nil, err
		}

		taskExecutionRole := fmt.Sprintf("%sTaskExecutionRole", service.Name)
		template.Resources[taskExecutionRole] = &iam.Role{
			AssumeRolePolicyDocument: assumeRolePolicyDocument,
			// Here we can grant access to secrets/configs using a Policy { Allow,ssm:GetParameters,secret|config ARN}
			ManagedPolicyArns: []string{
				ECSTaskExecutionPolicy,
			},
		}
		definition.ExecutionRoleArn = cloudformation.Ref(taskExecutionRole)
		// FIXME definition.TaskRoleArn = ?

		taskDefinition := fmt.Sprintf("%sTaskDefinition", service.Name)
		template.Resources[taskDefinition] = definition

		template.Resources[fmt.Sprintf("%sService", service.Name)] = &ecs.Service{
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

		var healthCheck *cloudmap.Service_HealthCheckConfig
		if service.HealthCheck != nil && !service.HealthCheck.Disable {
			// FIXME ECS only support HTTP(s) health checks, while Docker only support CMD
		}

		serviceRegistration := fmt.Sprintf("%sServiceRegistration", service.Name)
		template.Resources[serviceRegistration] = &cloudmap.Service{
			Description:       fmt.Sprintf("%q registration in Service Map", service.Name),
			HealthCheckConfig: healthCheck,
			Name:              serviceRegistration,
			NamespaceId:       cloudformation.Ref("CloudMap"),
		}
	}
	return template, nil
}

func (c client) GetVPC(ctx context.Context, project *compose.Project) (string, error) {
	//check compose file for the default external network
	if net, ok := project.Networks["default"]; ok {
		if net.External.External {
			vpc := net.Name
			ok, err := c.api.VpcExists(ctx, vpc)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", errors.New("Vpc does not exist: " + vpc)
			}
			return vpc, nil
		}
	}
	defaultVPC, err := c.api.GetDefaultVPC(ctx)
	if err != nil {
		return "", err
	}
	return defaultVPC, nil
}

type convertAPI interface {
	GetDefaultVPC(ctx context.Context) (string, error)
	VpcExists(ctx context.Context, vpcID string) (bool, error)
	GetSubNets(ctx context.Context, vpcID string) ([]string, error)
	GetRoleArn(ctx context.Context, name string) (string, error)
}
