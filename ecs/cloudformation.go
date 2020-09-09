/*
   Copyright 2020 Docker, Inc.

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
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/docker/compose-cli/api/compose"

	ecsapi "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
	cloudmapapi "github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/ec2"
	"github.com/awslabs/goformation/v4/cloudformation/ecs"
	"github.com/awslabs/goformation/v4/cloudformation/elasticloadbalancingv2"
	"github.com/awslabs/goformation/v4/cloudformation/iam"
	"github.com/awslabs/goformation/v4/cloudformation/logs"
	"github.com/awslabs/goformation/v4/cloudformation/secretsmanager"
	cloudmap "github.com/awslabs/goformation/v4/cloudformation/servicediscovery"
	"github.com/awslabs/goformation/v4/cloudformation/tags"
	"github.com/compose-spec/compose-go/compatibility"
	"github.com/compose-spec/compose-go/errdefs"
	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

const (
	parameterClusterName     = "ParameterClusterName"
	parameterVPCId           = "ParameterVPCId"
	parameterSubnet1Id       = "ParameterSubnet1Id"
	parameterSubnet2Id       = "ParameterSubnet2Id"
	parameterLoadBalancerARN = "ParameterLoadBalancerARN"
)

func (b *ecsAPIService) Convert(ctx context.Context, project *types.Project) ([]byte, error) {
	template, err := b.convert(project)
	if err != nil {
		return nil, err
	}
	return marshall(template)
}

// Convert a compose project into a CloudFormation template
func (b *ecsAPIService) convert(project *types.Project) (*cloudformation.Template, error) { //nolint:gocyclo
	var checker compatibility.Checker = &fargateCompatibilityChecker{
		compatibility.AllowList{
			Supported: compatibleComposeAttributes,
		},
	}
	compatibility.Check(project, checker)
	for _, err := range checker.Errors() {
		if errdefs.IsIncompatibleError(err) {
			logrus.Error(err.Error())
		} else {
			logrus.Warn(err.Error())
		}
	}
	if !compatibility.IsCompatible(checker) {
		return nil, fmt.Errorf("compose file is incompatible with Amazon ECS")
	}

	template := cloudformation.NewTemplate()
	template.Description = "CloudFormation template created by Docker for deploying applications on Amazon ECS"
	template.Parameters[parameterClusterName] = cloudformation.Parameter{
		Type:        "String",
		Description: "Name of the ECS cluster to deploy to (optional)",
	}

	template.Parameters[parameterVPCId] = cloudformation.Parameter{
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
	template.Parameters[parameterSubnet1Id] = cloudformation.Parameter{
		Type:        "AWS::EC2::Subnet::Id",
		Description: "SubnetId, for Availability Zone 1 in the region in your VPC",
	}
	template.Parameters[parameterSubnet2Id] = cloudformation.Parameter{
		Type:        "AWS::EC2::Subnet::Id",
		Description: "SubnetId, for Availability Zone 2 in the region in your VPC",
	}

	template.Parameters[parameterLoadBalancerARN] = cloudformation.Parameter{
		Type:        "String",
		Description: "Name of the LoadBalancer to connect to (optional)",
	}

	// Create Cluster is `ParameterClusterName` parameter is not set
	template.Conditions["CreateCluster"] = cloudformation.Equals("", cloudformation.Ref(parameterClusterName))

	cluster := createCluster(project, template)

	networks := map[string]string{}
	for _, net := range project.Networks {
		networks[net.Name] = convertNetwork(project, net, cloudformation.Ref(parameterVPCId), template)
	}

	for i, s := range project.Secrets {
		if s.External.External {
			continue
		}
		secret, err := ioutil.ReadFile(s.File)
		if err != nil {
			return nil, err
		}

		name := fmt.Sprintf("%sSecret", normalizeResourceName(s.Name))
		template.Resources[name] = &secretsmanager.Secret{
			Description:  "",
			SecretString: string(secret),
			Tags: []tags.Tag{
				{
					Key:   compose.ProjectTag,
					Value: project.Name,
				},
			},
		}
		s.Name = cloudformation.Ref(name)
		project.Secrets[i] = s
	}

	createLogGroup(project, template)

	// Private DNS namespace will allow DNS name for the services to be <service>.<project>.local
	createCloudMap(project, template)

	loadBalancerARN := createLoadBalancer(project, template)

	for _, service := range project.Services {

		definition, err := convert(project, service)
		if err != nil {
			return nil, err
		}

		taskExecutionRole := createTaskExecutionRole(service, definition, template)
		definition.ExecutionRoleArn = cloudformation.Ref(taskExecutionRole)

		taskRole := createTaskRole(service, template)
		if taskRole != "" {
			definition.TaskRoleArn = cloudformation.Ref(taskRole)
		}

		taskDefinition := fmt.Sprintf("%sTaskDefinition", normalizeResourceName(service.Name))
		template.Resources[taskDefinition] = definition

		var healthCheck *cloudmap.Service_HealthCheckConfig

		serviceRegistry := createServiceRegistry(service, template, healthCheck)

		serviceSecurityGroups := []string{}
		for net := range service.Networks {
			serviceSecurityGroups = append(serviceSecurityGroups, networks[net])
		}

		dependsOn := []string{}
		serviceLB := []ecs.Service_LoadBalancer{}
		if len(service.Ports) > 0 {
			for _, port := range service.Ports {
				protocol := strings.ToUpper(port.Protocol)
				if getLoadBalancerType(project) == elbv2.LoadBalancerTypeEnumApplication {
					protocol = elbv2.ProtocolEnumHttps
					if port.Published == 80 {
						protocol = elbv2.ProtocolEnumHttp
					}
				}
				if loadBalancerARN != "" {
					targetGroupName := createTargetGroup(project, service, port, template, protocol)
					listenerName := createListener(service, port, template, targetGroupName, loadBalancerARN, protocol)
					dependsOn = append(dependsOn, listenerName)
					serviceLB = append(serviceLB, ecs.Service_LoadBalancer{
						ContainerName:  service.Name,
						ContainerPort:  int(port.Target),
						TargetGroupArn: cloudformation.Ref(targetGroupName),
					})
				}
			}
		}

		desiredCount := 1
		if service.Deploy != nil && service.Deploy.Replicas != nil {
			desiredCount = int(*service.Deploy.Replicas)
		}

		for dependency := range service.DependsOn {
			dependsOn = append(dependsOn, serviceResourceName(dependency))
		}

		minPercent, maxPercent, err := computeRollingUpdateLimits(service)
		if err != nil {
			return nil, err
		}

		template.Resources[serviceResourceName(service.Name)] = &ecs.Service{
			AWSCloudFormationDependsOn: dependsOn,
			Cluster:                    cluster,
			DesiredCount:               desiredCount,
			DeploymentController: &ecs.Service_DeploymentController{
				Type: ecsapi.DeploymentControllerTypeEcs,
			},
			DeploymentConfiguration: &ecs.Service_DeploymentConfiguration{
				MaximumPercent:        maxPercent,
				MinimumHealthyPercent: minPercent,
			},
			LaunchType:    ecsapi.LaunchTypeFargate,
			LoadBalancers: serviceLB,
			NetworkConfiguration: &ecs.Service_NetworkConfiguration{
				AwsvpcConfiguration: &ecs.Service_AwsVpcConfiguration{
					AssignPublicIp: ecsapi.AssignPublicIpEnabled,
					SecurityGroups: serviceSecurityGroups,
					Subnets: []string{
						cloudformation.Ref(parameterSubnet1Id),
						cloudformation.Ref(parameterSubnet2Id),
					},
				},
			},
			PropagateTags:      ecsapi.PropagateTagsService,
			SchedulingStrategy: ecsapi.SchedulingStrategyReplica,
			ServiceRegistries:  []ecs.Service_ServiceRegistry{serviceRegistry},
			Tags: []tags.Tag{
				{
					Key:   compose.ProjectTag,
					Value: project.Name,
				},
				{
					Key:   compose.ServiceTag,
					Value: service.Name,
				},
			},
			TaskDefinition: cloudformation.Ref(normalizeResourceName(taskDefinition)),
		}
	}
	return template, nil
}

func createLogGroup(project *types.Project, template *cloudformation.Template) {
	retention := 0
	if v, ok := project.Extensions[extensionRetention]; ok {
		retention = v.(int)
	}
	logGroup := fmt.Sprintf("/docker-compose/%s", project.Name)
	template.Resources["LogGroup"] = &logs.LogGroup{
		LogGroupName:    logGroup,
		RetentionInDays: retention,
	}
}

func computeRollingUpdateLimits(service types.ServiceConfig) (int, int, error) {
	maxPercent := 200
	minPercent := 100
	if service.Deploy == nil || service.Deploy.UpdateConfig == nil {
		return minPercent, maxPercent, nil
	}
	updateConfig := service.Deploy.UpdateConfig
	min, okMin := updateConfig.Extensions[extensionMinPercent]
	if okMin {
		minPercent = min.(int)
	}
	max, okMax := updateConfig.Extensions[extensionMaxPercent]
	if okMax {
		maxPercent = max.(int)
	}
	if okMin && okMax {
		return minPercent, maxPercent, nil
	}

	if updateConfig.Parallelism != nil {
		parallelism := int(*updateConfig.Parallelism)
		if service.Deploy.Replicas == nil {
			return minPercent, maxPercent,
				fmt.Errorf("rolling update configuration require deploy.replicas to be set")
		}
		replicas := int(*service.Deploy.Replicas)
		if replicas < parallelism {
			return minPercent, maxPercent,
				fmt.Errorf("deploy.replicas (%d) must be greater than deploy.update_config.parallelism (%d)", replicas, parallelism)
		}
		if !okMin {
			minPercent = (replicas - parallelism) * 100 / replicas
		}
		if !okMax {
			maxPercent = (replicas + parallelism) * 100 / replicas
		}
	}
	return minPercent, maxPercent, nil
}

func getLoadBalancerType(project *types.Project) string {
	for _, service := range project.Services {
		for _, port := range service.Ports {
			protocol := port.Protocol
			v, ok := port.Extensions[extensionProtocol]
			if ok {
				protocol = v.(string)
			}
			if protocol == "http" || protocol == "https" {
				continue
			}
			if port.Published != 80 && port.Published != 443 {
				return elbv2.LoadBalancerTypeEnumNetwork
			}
		}
	}
	return elbv2.LoadBalancerTypeEnumApplication
}

func getLoadBalancerSecurityGroups(project *types.Project, template *cloudformation.Template) []string {
	securityGroups := []string{}
	for _, network := range project.Networks {
		if !network.Internal {
			net := convertNetwork(project, network, cloudformation.Ref(parameterVPCId), template)
			securityGroups = append(securityGroups, net)
		}
	}
	return uniqueStrings(securityGroups)
}

func createLoadBalancer(project *types.Project, template *cloudformation.Template) string {
	ports := 0
	for _, service := range project.Services {
		ports += len(service.Ports)
	}
	if ports == 0 {
		// Project do not expose any port (batch jobs?)
		// So no need to create a PortPublisher
		return ""
	}

	// load balancer names are limited to 32 characters total
	loadBalancerName := fmt.Sprintf("%.32s", fmt.Sprintf("%sLoadBalancer", strings.Title(project.Name)))
	// Create PortPublisher if `ParameterLoadBalancerName` is not set
	template.Conditions["CreateLoadBalancer"] = cloudformation.Equals("", cloudformation.Ref(parameterLoadBalancerARN))

	loadBalancerType := getLoadBalancerType(project)
	securityGroups := []string{}
	if loadBalancerType == elbv2.LoadBalancerTypeEnumApplication {
		securityGroups = getLoadBalancerSecurityGroups(project, template)
	}

	template.Resources[loadBalancerName] = &elasticloadbalancingv2.LoadBalancer{
		Name:           loadBalancerName,
		Scheme:         elbv2.LoadBalancerSchemeEnumInternetFacing,
		SecurityGroups: securityGroups,
		Subnets: []string{
			cloudformation.Ref(parameterSubnet1Id),
			cloudformation.Ref(parameterSubnet2Id),
		},
		Tags: []tags.Tag{
			{
				Key:   compose.ProjectTag,
				Value: project.Name,
			},
		},
		Type:                       loadBalancerType,
		AWSCloudFormationCondition: "CreateLoadBalancer",
	}
	return cloudformation.If("CreateLoadBalancer", cloudformation.Ref(loadBalancerName), cloudformation.Ref(parameterLoadBalancerARN))
}

func createListener(service types.ServiceConfig, port types.ServicePortConfig, template *cloudformation.Template, targetGroupName string, loadBalancerARN string, protocol string) string {
	listenerName := fmt.Sprintf(
		"%s%s%dListener",
		normalizeResourceName(service.Name),
		strings.ToUpper(port.Protocol),
		port.Target,
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
		LoadBalancerArn: loadBalancerARN,
		Protocol:        protocol,
		Port:            int(port.Target),
	}
	return listenerName
}

func createTargetGroup(project *types.Project, service types.ServiceConfig, port types.ServicePortConfig, template *cloudformation.Template, protocol string) string {
	targetGroupName := fmt.Sprintf(
		"%s%s%dTargetGroup",
		normalizeResourceName(service.Name),
		strings.ToUpper(port.Protocol),
		port.Published,
	)
	template.Resources[targetGroupName] = &elasticloadbalancingv2.TargetGroup{
		Port:     int(port.Target),
		Protocol: protocol,
		Tags: []tags.Tag{
			{
				Key:   compose.ProjectTag,
				Value: project.Name,
			},
		},
		VpcId:      cloudformation.Ref(parameterVPCId),
		TargetType: elbv2.TargetTypeEnumIp,
	}
	return targetGroupName
}

func createServiceRegistry(service types.ServiceConfig, template *cloudformation.Template, healthCheck *cloudmap.Service_HealthCheckConfig) ecs.Service_ServiceRegistry {
	serviceRegistration := fmt.Sprintf("%sServiceDiscoveryEntry", normalizeResourceName(service.Name))
	serviceRegistry := ecs.Service_ServiceRegistry{
		RegistryArn: cloudformation.GetAtt(serviceRegistration, "Arn"),
	}

	template.Resources[serviceRegistration] = &cloudmap.Service{
		Description:       fmt.Sprintf("%q service discovery entry in Cloud Map", service.Name),
		HealthCheckConfig: healthCheck,
		HealthCheckCustomConfig: &cloudmap.Service_HealthCheckCustomConfig{
			FailureThreshold: 1,
		},
		Name:        service.Name,
		NamespaceId: cloudformation.Ref("CloudMap"),
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

func createTaskExecutionRole(service types.ServiceConfig, definition *ecs.TaskDefinition, template *cloudformation.Template) string {
	taskExecutionRole := fmt.Sprintf("%sTaskExecutionRole", normalizeResourceName(service.Name))
	policies := createPolicies(service, definition)
	template.Resources[taskExecutionRole] = &iam.Role{
		AssumeRolePolicyDocument: assumeRolePolicyDocument,
		Policies:                 policies,
		ManagedPolicyArns: []string{
			ecsTaskExecutionPolicy,
			ecrReadOnlyPolicy,
		},
	}
	return taskExecutionRole
}

func createTaskRole(service types.ServiceConfig, template *cloudformation.Template) string {
	taskRole := fmt.Sprintf("%sTaskRole", normalizeResourceName(service.Name))
	rolePolicies := []iam.Role_Policy{}
	if roles, ok := service.Extensions[extensionRole]; ok {
		rolePolicies = append(rolePolicies, iam.Role_Policy{
			PolicyDocument: roles,
		})
	}
	managedPolicies := []string{}
	if v, ok := service.Extensions[extensionManagedPolicies]; ok {
		for _, s := range v.([]interface{}) {
			managedPolicies = append(managedPolicies, s.(string))
		}
	}
	if len(rolePolicies) == 0 && len(managedPolicies) == 0 {
		return ""
	}
	template.Resources[taskRole] = &iam.Role{
		AssumeRolePolicyDocument: assumeRolePolicyDocument,
		Policies:                 rolePolicies,
		ManagedPolicyArns:        managedPolicies,
	}
	return taskRole
}

func createCluster(project *types.Project, template *cloudformation.Template) string {
	template.Resources["Cluster"] = &ecs.Cluster{
		ClusterName: project.Name,
		Tags: []tags.Tag{
			{
				Key:   compose.ProjectTag,
				Value: project.Name,
			},
		},
		AWSCloudFormationCondition: "CreateCluster",
	}
	cluster := cloudformation.If("CreateCluster", cloudformation.Ref("Cluster"), cloudformation.Ref(parameterClusterName))
	return cluster
}

func createCloudMap(project *types.Project, template *cloudformation.Template) {
	template.Resources["CloudMap"] = &cloudmap.PrivateDnsNamespace{
		Description: fmt.Sprintf("Service Map for Docker Compose project %s", project.Name),
		Name:        fmt.Sprintf("%s.local", project.Name),
		Vpc:         cloudformation.Ref(parameterVPCId),
	}
}

func convertNetwork(project *types.Project, net types.NetworkConfig, vpc string, template *cloudformation.Template) string {
	if sg, ok := net.Extensions[extensionSecurityGroup]; ok {
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
				Key:   compose.ProjectTag,
				Value: project.Name,
			},
			{
				Key:   compose.NetworkTag,
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

func networkResourceName(project *types.Project, network string) string {
	return fmt.Sprintf("%s%sNetwork", normalizeResourceName(project.Name), normalizeResourceName(network))
}

func serviceResourceName(dependency string) string {
	return fmt.Sprintf("%sService", normalizeResourceName(dependency))
}

func normalizeResourceName(s string) string {
	return strings.Title(regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(s, ""))
}

func createPolicies(service types.ServiceConfig, taskDef *ecs.TaskDefinition) []iam.Role_Policy {
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
		return []iam.Role_Policy{
			{
				PolicyDocument: &PolicyDocument{
					Statement: []PolicyStatement{
						{
							Effect:   "Allow",
							Action:   []string{actionGetSecretValue, actionGetParameters, actionDecrypt},
							Resource: arns,
						},
					},
				},
				PolicyName: fmt.Sprintf("%sGrantAccessToSecrets", service.Name),
			},
		}
	}
	return nil
}

func uniqueStrings(items []string) []string {
	keys := make(map[string]bool)
	unique := []string{}
	for _, item := range items {
		if _, val := keys[item]; !val {
			keys[item] = true
			unique = append(unique, item)
		}
	}
	return unique
}
