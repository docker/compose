package amazon

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/types"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/service/elbv2"
	cloudmapapi "github.com/aws/aws-sdk-go/service/servicediscovery"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/awslabs/goformation/v4/cloudformation/logs"
	cloudmap "github.com/awslabs/goformation/v4/cloudformation/servicediscovery"
	"github.com/awslabs/goformation/v4/cloudformation/tags"
	"github.com/docker/ecs-plugin/pkg/compose"
)

const (
	ParameterClusterName = "ParameterClusterName"
	ParameterVPCId       = "ParameterVPCId"
	ParameterSubnet1Id   = "ParameterSubnet1Id"
	ParameterSubnet2Id   = "ParameterSubnet2Id"
)

// Convert a compose project into a CloudFormation template
func (c client) Convert(project *compose.Project) (*cloudformation.Template, error) {
	warnings := Check(project)
	for _, w := range warnings {
		logrus.Warn(w)
	}

	template := cloudformation.NewTemplate()

	template.Parameters[ParameterClusterName] = cloudformation.Parameter{
		Type:        "String",
		Description: "Name of the ECS cluster to deploy to (optional)",
	}

	template.Parameters[ParameterVPCId] = cloudformation.Parameter{
		Type:        "AWS::EC2::VPC::Id",
		Description: "ID of the VPC",
	}

	/*
		FIXME can't set subnets: Ref("SubnetIds") see https://github.com/awslabs/goformation/issues/282
		template.Parameters["SubnetIds"] = cloudformation.Parameter{
			Type:        "List<AWS::EC2::Subnet::Id>",
			Description: "The list of SubnetIds, for at least two Availability Zones in the region in your VPC",
		}
	*/
	template.Parameters[ParameterSubnet1Id] = cloudformation.Parameter{
		Type:        "AWS::EC2::Subnet::Id",
		Description: "SubnetId, for Availability Zone 1 in the region in your VPC",
	}
	template.Parameters[ParameterSubnet2Id] = cloudformation.Parameter{
		Type:        "AWS::EC2::Subnet::Id",
		Description: "SubnetId, for Availability Zone 2 in the region in your VPC",
	}

	// Create Cluster is `ParameterClusterName` parameter is not set
	template.Conditions["CreateCluster"] = cloudformation.Equals("", cloudformation.Ref(ParameterClusterName))

	cluster := c.createCluster(project, template)

	networks := map[string]string{}
	for _, net := range project.Networks {
		networks[net.Name] = convertNetwork(project, net, cloudformation.Ref(ParameterVPCId), template)
	}

	logGroup := fmt.Sprintf("/docker-compose/%s", project.Name)
	template.Resources["LogGroup"] = &logs.LogGroup{
		LogGroupName: logGroup,
	}

	// Private DNS namespace will allow DNS name for the services to be <service>.<project>.local
	c.createCloudMap(project, template)
	loadBalancer := c.createLoadBalancer(project, template)

	for _, service := range project.Services {
		definition, err := Convert(project, service)
		if err != nil {
			return nil, err
		}

		taskExecutionRole, err := c.createTaskExecutionRole(service, err, definition, template)
		if err != nil {
			return template, err
		}
		definition.ExecutionRoleArn = cloudformation.Ref(taskExecutionRole)

		taskDefinition := fmt.Sprintf("%sTaskDefinition", normalizeResourceName(service.Name))
		template.Resources[taskDefinition] = definition

		var healthCheck *cloudmap.Service_HealthCheckConfig
		if service.HealthCheck != nil && !service.HealthCheck.Disable {
			// FIXME ECS only support HTTP(s) health checks, while Docker only support CMD
		}

		serviceRegistry := c.createServiceRegistry(service, template, healthCheck)

		serviceSecurityGroups := []string{}
		for net := range service.Networks {
			serviceSecurityGroups = append(serviceSecurityGroups, networks[net])
		}

		dependsOn := []string{}
		serviceLB := []ecs.Service_LoadBalancer{}
		if len(service.Ports) > 0 {
			for _, port := range service.Ports {
				protocol := strings.ToUpper(port.Protocol)
				targetGroupName := c.createTargetGroup(project, service, port, template, protocol)
				listenerName := c.createListener(service, port, template, targetGroupName, loadBalancer, protocol)
				dependsOn = append(dependsOn, listenerName)
				serviceLB = append(serviceLB, ecs.Service_LoadBalancer{
					ContainerName:  service.Name,
					ContainerPort:  int(port.Published),
					TargetGroupArn: cloudformation.Ref(targetGroupName),
				})
			}
		}

		desiredCount := 1
		if service.Deploy != nil && service.Deploy.Replicas != nil {
			desiredCount = int(*service.Deploy.Replicas)
		}

		for _, dependency := range service.DependsOn {
			dependsOn = append(dependsOn, serviceResourceName(dependency))
		}
		template.Resources[serviceResourceName(service.Name)] = &ecs.Service{
			AWSCloudFormationDependsOn: dependsOn,
			Cluster:                    cluster,
			DesiredCount:               desiredCount,
			LaunchType:                 ecsapi.LaunchTypeFargate,
			LoadBalancers:              serviceLB,
			NetworkConfiguration: &ecs.Service_NetworkConfiguration{
				AwsvpcConfiguration: &ecs.Service_AwsVpcConfiguration{
					AssignPublicIp: ecsapi.AssignPublicIpEnabled,
					SecurityGroups: serviceSecurityGroups,
					Subnets: []string{
						cloudformation.Ref(ParameterSubnet1Id),
						cloudformation.Ref(ParameterSubnet2Id),
					},
				},
			},
			SchedulingStrategy: ecsapi.SchedulingStrategyReplica,
			ServiceName:        service.Name,
			ServiceRegistries:  []ecs.Service_ServiceRegistry{serviceRegistry},
			Tags: []tags.Tag{
				{
					Key:   ProjectTag,
					Value: project.Name,
				},
				{
					Key:   ServiceTag,
					Value: service.Name,
				},
			},
			TaskDefinition: cloudformation.Ref(normalizeResourceName(taskDefinition)),
		}
	}
	return template, nil
}

func (c client) createLoadBalancer(project *compose.Project, template *cloudformation.Template) string {
	loadBalancerName := fmt.Sprintf("%sLoadBalancer", strings.Title(project.Name))
	template.Resources[loadBalancerName] = &elasticloadbalancingv2.LoadBalancer{
		Name:   loadBalancerName,
		Scheme: elbv2.LoadBalancerSchemeEnumInternetFacing,
		Subnets: []string{
			cloudformation.Ref(ParameterSubnet1Id),
			cloudformation.Ref(ParameterSubnet2Id),
		},
		Tags: []tags.Tag{
			{
				Key:   ProjectTag,
				Value: project.Name,
			},
		},
		Type: elbv2.LoadBalancerTypeEnumNetwork,
	}
	return loadBalancerName
}

func (c client) createListener(service types.ServiceConfig, port types.ServicePortConfig, template *cloudformation.Template, targetGroupName string, loadBalancerName string, protocol string) string {
	listenerName := fmt.Sprintf(
		"%s%s%dListener",
		normalizeResourceName(service.Name),
		strings.ToUpper(port.Protocol),
		port.Published,
	)
	//add listener to dependsOn
	//https://stackoverflow.com/questions/53971873/the-target-group-does-not-have-an-associated-load-balancer
	template.Resources[listenerName] = &elasticloadbalancingv2.Listener{
		DefaultActions: []elasticloadbalancingv2.Listener_Action{
			{
				ForwardConfig: &elasticloadbalancingv2.Listener_ForwardConfig{
					TargetGroups: []elasticloadbalancingv2.Listener_TargetGroupTuple{
						{
							TargetGroupArn: cloudformation.Ref(targetGroupName),
						},
					},
				},
				Type: elbv2.ActionTypeEnumForward,
			},
		},
		LoadBalancerArn: cloudformation.Ref(loadBalancerName),
		Protocol:        protocol,
		Port:            int(port.Published),
	}
	return listenerName
}

func (c client) createTargetGroup(project *compose.Project, service types.ServiceConfig, port types.ServicePortConfig, template *cloudformation.Template, protocol string) string {
	targetGroupName := fmt.Sprintf(
		"%s%s%dTargetGroup",
		normalizeResourceName(service.Name),
		strings.ToUpper(port.Protocol),
		port.Published,
	)
	template.Resources[targetGroupName] = &elasticloadbalancingv2.TargetGroup{
		Name:     targetGroupName,
		Port:     int(port.Target),
		Protocol: protocol,
		Tags: []tags.Tag{
			{
				Key:   ProjectTag,
				Value: project.Name,
			},
		},
		VpcId:      cloudformation.Ref(ParameterVPCId),
		TargetType: elbv2.TargetTypeEnumIp,
	}
	return targetGroupName
}

func (c client) createServiceRegistry(service types.ServiceConfig, template *cloudformation.Template, healthCheck *cloudmap.Service_HealthCheckConfig) ecs.Service_ServiceRegistry {
	serviceRegistration := fmt.Sprintf("%sServiceDiscoveryEntry", normalizeResourceName(service.Name))
	serviceRegistry := ecs.Service_ServiceRegistry{
		RegistryArn: cloudformation.GetAtt(serviceRegistration, "Arn"),
	}

	template.Resources[serviceRegistration] = &cloudmap.Service{
		Description:       fmt.Sprintf("%q service discovery entry in Cloud Map", service.Name),
		HealthCheckConfig: healthCheck,
		Name:              service.Name,
		NamespaceId:       cloudformation.Ref("CloudMap"),
		DnsConfig: &cloudmap.Service_DnsConfig{
			DnsRecords: []cloudmap.Service_DnsRecord{
				{
					TTL:  60,
					Type: cloudmapapi.RecordTypeA,
				},
			},
			RoutingPolicy: cloudmapapi.RoutingPolicyMultivalue,
		},
	}
	return serviceRegistry
}

func (c client) createTaskExecutionRole(service types.ServiceConfig, err error, definition *ecs.TaskDefinition, template *cloudformation.Template) (string, error) {
	taskExecutionRole := fmt.Sprintf("%sTaskExecutionRole", normalizeResourceName(service.Name))
	policy, err := c.getPolicy(definition)
	if err != nil {
		return taskExecutionRole, err
	}
	rolePolicies := []iam.Role_Policy{}
	if policy != nil {
		rolePolicies = append(rolePolicies, iam.Role_Policy{
			PolicyDocument: policy,
			PolicyName:     fmt.Sprintf("%sGrantAccessToSecrets", service.Name),
		})

	}
	template.Resources[taskExecutionRole] = &iam.Role{
		AssumeRolePolicyDocument: assumeRolePolicyDocument,
		Policies:                 rolePolicies,
		ManagedPolicyArns: []string{
			ECSTaskExecutionPolicy,
			ECRReadOnlyPolicy,
		},
	}
	return taskExecutionRole, nil
}

func (c client) createCluster(project *compose.Project, template *cloudformation.Template) string {
	template.Resources["Cluster"] = &ecs.Cluster{
		ClusterName: project.Name,
		Tags: []tags.Tag{
			{
				Key:   ProjectTag,
				Value: project.Name,
			},
		},
		AWSCloudFormationCondition: "CreateCluster",
	}
	cluster := cloudformation.If("CreateCluster", cloudformation.Ref("Cluster"), cloudformation.Ref(ParameterClusterName))
	return cluster
}

func (c client) createCloudMap(project *compose.Project, template *cloudformation.Template) {
	template.Resources["CloudMap"] = &cloudmap.PrivateDnsNamespace{
		Description: fmt.Sprintf("Service Map for Docker Compose project %s", project.Name),
		Name:        fmt.Sprintf("%s.local", project.Name),
		Vpc:         cloudformation.Ref(ParameterVPCId),
	}
}

func convertNetwork(project *compose.Project, net types.NetworkConfig, vpc string, template *cloudformation.Template) string {
	if sg, ok := net.Extras[ExtensionSecurityGroup]; ok {
		logrus.Debugf("Security Group for network %q set by user to %q", net.Name, sg)
		return sg.(string)
	}

	var ingresses []ec2.SecurityGroup_Ingress
	if !net.Internal {
		for _, service := range project.Services {
			if _, ok := service.Networks[net.Name]; ok {
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
		}
	}

	securityGroup := networkResourceName(project, net.Name)
	template.Resources[securityGroup] = &ec2.SecurityGroup{
		GroupDescription:     fmt.Sprintf("%s %s Security Group", project.Name, net.Name),
		GroupName:            securityGroup,
		SecurityGroupIngress: ingresses,
		VpcId:                vpc,
		Tags: []tags.Tag{
			{
				Key:   ProjectTag,
				Value: project.Name,
			},
			{
				Key:   NetworkTag,
				Value: net.Name,
			},
		},
	}

	ingress := securityGroup + "Ingress"
	template.Resources[ingress] = &ec2.SecurityGroupIngress{
		Description:           fmt.Sprintf("Allow communication within network %s", net.Name),
		IpProtocol:            "-1", // all protocols
		GroupId:               cloudformation.Ref(securityGroup),
		SourceSecurityGroupId: cloudformation.Ref(securityGroup),
	}

	return cloudformation.Ref(securityGroup)
}

func networkResourceName(project *compose.Project, network string) string {
	return fmt.Sprintf("%s%sNetwork", normalizeResourceName(project.Name), normalizeResourceName(network))
}

func serviceResourceName(dependency string) string {
	return fmt.Sprintf("%sService", normalizeResourceName(dependency))
}

func normalizeResourceName(s string) string {
	return strings.Title(regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(s, ""))
}

func (c client) getPolicy(taskDef *ecs.TaskDefinition) (*PolicyDocument, error) {

	arns := []string{}
	for _, container := range taskDef.ContainerDefinitions {
		if container.RepositoryCredentials != nil {
			arns = append(arns, container.RepositoryCredentials.CredentialsParameter)
		}
		if len(container.Secrets) > 0 {
			for _, s := range container.Secrets {
				arns = append(arns, s.ValueFrom)
			}
		}

	}
	if len(arns) > 0 {
		return &PolicyDocument{
			Statement: []PolicyStatement{
				{
					Effect:   "Allow",
					Action:   []string{ActionGetSecretValue, ActionGetParameters, ActionDecrypt},
					Resource: arns,
				}},
		}, nil
	}
	return nil, nil
}
