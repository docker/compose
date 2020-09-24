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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/secrets"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/aws/aws-sdk-go/service/efs/efsiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/sirupsen/logrus"
)

type sdk struct {
	ECS ecsiface.ECSAPI
	EC2 ec2iface.EC2API
	EFS efsiface.EFSAPI
	ELB elbv2iface.ELBV2API
	CW  cloudwatchlogsiface.CloudWatchLogsAPI
	IAM iamiface.IAMAPI
	CF  cloudformationiface.CloudFormationAPI
	SM  secretsmanageriface.SecretsManagerAPI
	SSM ssmiface.SSMAPI
}

func newSDK(sess *session.Session) sdk {
	sess.Handlers.Build.PushBack(func(r *request.Request) {
		request.AddToUserAgent(r, "Docker CLI")
	})
	return sdk{
		ECS: ecs.New(sess),
		EC2: ec2.New(sess),
		EFS: efs.New(sess),
		ELB: elbv2.New(sess),
		CW:  cloudwatchlogs.New(sess),
		IAM: iam.New(sess),
		CF:  cloudformation.New(sess),
		SM:  secretsmanager.New(sess),
		SSM: ssm.New(sess),
	}
}

func (s sdk) CheckRequirements(ctx context.Context, region string) error {
	settings, err := s.ECS.ListAccountSettingsWithContext(ctx, &ecs.ListAccountSettingsInput{
		EffectiveSettings: aws.Bool(true),
		Name:              aws.String("serviceLongArnFormat"),
	})
	if err != nil {
		return err
	}
	serviceLongArnFormat := settings.Settings[0].Value
	if *serviceLongArnFormat != "enabled" {
		return fmt.Errorf("this tool requires the \"new ARN resource ID format\".\n"+
			"Check https://%s.console.aws.amazon.com/ecs/home#/settings\n"+
			"Learn more: https://aws.amazon.com/blogs/compute/migrating-your-amazon-ecs-deployment-to-the-new-arn-and-resource-id-format-2", region)
	}
	return nil
}

func (s sdk) ClusterExists(ctx context.Context, name string) (bool, error) {
	logrus.Debug("CheckRequirements if cluster was already created: ", name)
	clusters, err := s.ECS.DescribeClustersWithContext(ctx, &ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(name)},
	})
	if err != nil {
		return false, err
	}
	return len(clusters.Clusters) > 0, nil
}

func (s sdk) CreateCluster(ctx context.Context, name string) (string, error) {
	logrus.Debug("Create cluster ", name)
	response, err := s.ECS.CreateClusterWithContext(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(name)})
	if err != nil {
		return "", err
	}
	return *response.Cluster.Status, nil
}

func (s sdk) CheckVPC(ctx context.Context, vpcID string) error {
	logrus.Debug("CheckRequirements on VPC : ", vpcID)
	output, err := s.EC2.DescribeVpcAttributeWithContext(ctx, &ec2.DescribeVpcAttributeInput{
		VpcId:     aws.String(vpcID),
		Attribute: aws.String("enableDnsSupport"),
	})
	if err != nil {
		return err
	}
	if !*output.EnableDnsSupport.Value {
		return fmt.Errorf("VPC %q doesn't have DNS resolution enabled", vpcID)
	}
	return err
}

func (s sdk) GetDefaultVPC(ctx context.Context) (string, error) {
	logrus.Debug("Retrieve default VPC")
	vpcs, err := s.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(vpcs.Vpcs) == 0 {
		return "", fmt.Errorf("account has not default VPC")
	}
	return *vpcs.Vpcs[0].VpcId, nil
}

func (s sdk) GetSubNets(ctx context.Context, vpcID string) ([]string, error) {
	logrus.Debug("Retrieve SubNets")
	subnets, err := s.EC2.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
		DryRun: nil,
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	ids := []string{}
	for _, subnet := range subnets.Subnets {
		ids = append(ids, *subnet.SubnetId)
	}
	return ids, nil
}

func (s sdk) GetRoleArn(ctx context.Context, name string) (string, error) {
	role, err := s.IAM.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *role.Role.Arn, nil
}

func (s sdk) StackExists(ctx context.Context, name string) (bool, error) {
	stacks, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), fmt.Sprintf("ValidationError: Stack with ID %s does not exist", name)) {
			return false, nil
		}
		return false, nil
	}
	return len(stacks.Stacks) > 0, nil
}

func (s sdk) CreateStack(ctx context.Context, name string, template []byte) error {
	logrus.Debug("Create CloudFormation stack")

	_, err := s.CF.CreateStackWithContext(ctx, &cloudformation.CreateStackInput{
		OnFailure:        aws.String("DELETE"),
		StackName:        aws.String(name),
		TemplateBody:     aws.String(string(template)),
		TimeoutInMinutes: nil,
		Capabilities: []*string{
			aws.String(cloudformation.CapabilityCapabilityIam),
		},
		Tags: []*cloudformation.Tag{
			{
				Key:   aws.String(compose.ProjectTag),
				Value: aws.String(name),
			},
		},
	})
	return err
}

func (s sdk) CreateChangeSet(ctx context.Context, name string, template []byte) (string, error) {
	logrus.Debug("Create CloudFormation Changeset")

	update := fmt.Sprintf("Update%s", time.Now().Format("2006-01-02-15-04-05"))
	changeset, err := s.CF.CreateChangeSetWithContext(ctx, &cloudformation.CreateChangeSetInput{
		ChangeSetName: aws.String(update),
		ChangeSetType: aws.String(cloudformation.ChangeSetTypeUpdate),
		StackName:     aws.String(name),
		TemplateBody:  aws.String(string(template)),
		Capabilities: []*string{
			aws.String(cloudformation.CapabilityCapabilityIam),
		},
	})
	if err != nil {
		return "", err
	}

	err = s.CF.WaitUntilChangeSetCreateCompleteWithContext(ctx, &cloudformation.DescribeChangeSetInput{
		ChangeSetName: changeset.Id,
	})
	return *changeset.Id, err
}

func (s sdk) UpdateStack(ctx context.Context, changeset string) error {
	desc, err := s.CF.DescribeChangeSetWithContext(ctx, &cloudformation.DescribeChangeSetInput{
		ChangeSetName: aws.String(changeset),
	})
	if err != nil {
		return err
	}

	if strings.HasPrefix(aws.StringValue(desc.StatusReason), "The submitted information didn't contain changes.") {
		return nil
	}

	_, err = s.CF.ExecuteChangeSet(&cloudformation.ExecuteChangeSetInput{
		ChangeSetName: aws.String(changeset),
	})
	return err
}

const (
	stackCreate = iota
	stackUpdate
	stackDelete
)

func (s sdk) WaitStackComplete(ctx context.Context, name string, operation int) error {
	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	}
	switch operation {
	case stackCreate:
		return s.CF.WaitUntilStackCreateCompleteWithContext(ctx, input)
	case stackDelete:
		return s.CF.WaitUntilStackDeleteCompleteWithContext(ctx, input)
	default:
		return fmt.Errorf("internal error: unexpected stack operation %d", operation)
	}
}

func (s sdk) GetStackID(ctx context.Context, name string) (string, error) {
	stacks, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *stacks.Stacks[0].StackId, nil
}

func (s sdk) ListStacks(ctx context.Context, name string) ([]compose.Stack, error) {
	params := cloudformation.DescribeStacksInput{}
	if name != "" {
		params.StackName = &name
	}
	cfStacks, err := s.CF.DescribeStacksWithContext(ctx, &params)
	if err != nil {
		return nil, err
	}
	stacks := []compose.Stack{}
	for _, stack := range cfStacks.Stacks {
		for _, t := range stack.Tags {
			if *t.Key == compose.ProjectTag {
				status := compose.RUNNING
				switch aws.StringValue(stack.StackStatus) {
				case "CREATE_IN_PROGRESS":
					status = compose.STARTING
				case "DELETE_IN_PROGRESS":
					status = compose.REMOVING
				case "UPDATE_IN_PROGRESS":
					status = compose.UPDATING
				}
				stacks = append(stacks, compose.Stack{
					ID:     aws.StringValue(stack.StackId),
					Name:   aws.StringValue(stack.StackName),
					Status: status,
				})
				break
			}
		}
	}
	return stacks, nil
}

func (s sdk) DescribeStackEvents(ctx context.Context, stackID string) ([]*cloudformation.StackEvent, error) {
	// Fixme implement Paginator on Events and return as a chan(events)
	events := []*cloudformation.StackEvent{}
	var nextToken *string
	for {
		resp, err := s.CF.DescribeStackEventsWithContext(ctx, &cloudformation.DescribeStackEventsInput{
			StackName: aws.String(stackID),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, resp.StackEvents...)
		if resp.NextToken == nil {
			return events, nil
		}
		nextToken = resp.NextToken
	}
}

func (s sdk) ListStackParameters(ctx context.Context, name string) (map[string]string, error) {
	st, err := s.CF.DescribeStacksWithContext(ctx, &cloudformation.DescribeStacksInput{
		NextToken: nil,
		StackName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	parameters := map[string]string{}
	for _, parameter := range st.Stacks[0].Parameters {
		parameters[aws.StringValue(parameter.ParameterKey)] = aws.StringValue(parameter.ParameterValue)
	}
	return parameters, nil
}

type stackResource struct {
	LogicalID string
	Type      string
	ARN       string
	Status    string
}

func (s sdk) ListStackResources(ctx context.Context, name string) ([]stackResource, error) {
	// FIXME handle pagination
	res, err := s.CF.ListStackResourcesWithContext(ctx, &cloudformation.ListStackResourcesInput{
		StackName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}

	resources := []stackResource{}
	for _, r := range res.StackResourceSummaries {
		resources = append(resources, stackResource{
			LogicalID: aws.StringValue(r.LogicalResourceId),
			Type:      aws.StringValue(r.ResourceType),
			ARN:       aws.StringValue(r.PhysicalResourceId),
			Status:    aws.StringValue(r.ResourceStatus),
		})
	}
	return resources, nil
}

func (s sdk) DeleteStack(ctx context.Context, name string) error {
	logrus.Debug("Delete CloudFormation stack")
	_, err := s.CF.DeleteStackWithContext(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(name),
	})
	return err
}

func (s sdk) CreateSecret(ctx context.Context, secret secrets.Secret) (string, error) {
	logrus.Debug("Create secret " + secret.Name)
	secretStr, err := secret.GetCredString()
	if err != nil {
		return "", err
	}

	response, err := s.SM.CreateSecret(&secretsmanager.CreateSecretInput{
		Name:         &secret.Name,
		SecretString: &secretStr,
		Description:  &secret.Description,
	})
	if err != nil {
		return "", err
	}
	return aws.StringValue(response.ARN), nil
}

func (s sdk) InspectSecret(ctx context.Context, id string) (secrets.Secret, error) {
	logrus.Debug("Inspect secret " + id)
	response, err := s.SM.DescribeSecret(&secretsmanager.DescribeSecretInput{SecretId: &id})
	if err != nil {
		return secrets.Secret{}, err
	}
	labels := map[string]string{}
	for _, tag := range response.Tags {
		labels[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
	}
	secret := secrets.Secret{
		ID:     aws.StringValue(response.ARN),
		Name:   aws.StringValue(response.Name),
		Labels: labels,
	}
	if response.Description != nil {
		secret.Description = *response.Description
	}
	return secret, nil
}

func (s sdk) ListSecrets(ctx context.Context) ([]secrets.Secret, error) {
	logrus.Debug("List secrets ...")
	response, err := s.SM.ListSecrets(&secretsmanager.ListSecretsInput{})
	if err != nil {
		return nil, err
	}

	var ls []secrets.Secret
	for _, sec := range response.SecretList {

		labels := map[string]string{}
		for _, tag := range sec.Tags {
			labels[*tag.Key] = *tag.Value
		}
		description := ""
		if sec.Description != nil {
			description = *sec.Description
		}
		ls = append(ls, secrets.Secret{
			ID:          *sec.ARN,
			Name:        *sec.Name,
			Labels:      labels,
			Description: description,
		})
	}
	return ls, nil
}

func (s sdk) DeleteSecret(ctx context.Context, id string, recover bool) error {
	logrus.Debug("List secrets ...")
	force := !recover
	_, err := s.SM.DeleteSecret(&secretsmanager.DeleteSecretInput{SecretId: &id, ForceDeleteWithoutRecovery: &force})
	return err
}

func (s sdk) GetLogs(ctx context.Context, name string, consumer func(service, container, message string)) error {
	logGroup := fmt.Sprintf("/docker-compose/%s", name)
	var startTime int64
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var hasMore = true
			var token *string
			for hasMore {
				events, err := s.CW.FilterLogEvents(&cloudwatchlogs.FilterLogEventsInput{
					LogGroupName: aws.String(logGroup),
					NextToken:    token,
					StartTime:    aws.Int64(startTime),
				})
				if err != nil {
					return err
				}
				if events.NextToken == nil {
					hasMore = false
				} else {
					token = events.NextToken
				}

				for _, event := range events.Events {
					p := strings.Split(aws.StringValue(event.LogStreamName), "/")
					consumer(p[1], p[2], aws.StringValue(event.Message))
					startTime = *event.IngestionTime
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s sdk) DescribeServices(ctx context.Context, cluster string, arns []string) ([]compose.ServiceStatus, error) {
	services, err := s.ECS.DescribeServicesWithContext(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(cluster),
		Services: aws.StringSlice(arns),
		Include:  aws.StringSlice([]string{"TAGS"}),
	})
	if err != nil {
		return nil, err
	}

	status := []compose.ServiceStatus{}
	for _, service := range services.Services {
		var name string
		for _, t := range service.Tags {
			if *t.Key == compose.ServiceTag {
				name = aws.StringValue(t.Value)
			}
		}
		if name == "" {
			return nil, fmt.Errorf("service %s doesn't have a %s tag", *service.ServiceArn, compose.ServiceTag)
		}
		targetGroupArns := []string{}
		for _, lb := range service.LoadBalancers {
			targetGroupArns = append(targetGroupArns, *lb.TargetGroupArn)
		}
		// getURLwithPortMapping makes 2 queries
		// one to get the target groups and another for load balancers
		loadBalancers, err := s.getURLWithPortMapping(ctx, targetGroupArns)
		if err != nil {
			return nil, err
		}
		status = append(status, compose.ServiceStatus{
			ID:         aws.StringValue(service.ServiceName),
			Name:       name,
			Replicas:   int(aws.Int64Value(service.RunningCount)),
			Desired:    int(aws.Int64Value(service.DesiredCount)),
			Publishers: loadBalancers,
		})
	}
	return status, nil
}

func (s sdk) getURLWithPortMapping(ctx context.Context, targetGroupArns []string) ([]compose.PortPublisher, error) {
	if len(targetGroupArns) == 0 {
		return nil, nil
	}
	groups, err := s.ELB.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: aws.StringSlice(targetGroupArns),
	})
	if err != nil {
		return nil, err
	}
	lbarns := []*string{}
	for _, tg := range groups.TargetGroups {
		lbarns = append(lbarns, tg.LoadBalancerArns...)
	}

	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: lbarns,
	})

	if err != nil {
		return nil, err
	}
	filterLB := func(arn *string, lbs []*elbv2.LoadBalancer) *elbv2.LoadBalancer {
		if aws.StringValue(arn) == "" {
			// load balancer arn is nil/""
			return nil
		}
		for _, lb := range lbs {
			if aws.StringValue(lb.LoadBalancerArn) == aws.StringValue(arn) {
				return lb
			}
		}
		return nil
	}
	loadBalancers := []compose.PortPublisher{}
	for _, tg := range groups.TargetGroups {
		for _, lbarn := range tg.LoadBalancerArns {
			lb := filterLB(lbarn, lbs.LoadBalancers)
			if lb == nil {
				continue
			}
			loadBalancers = append(loadBalancers, compose.PortPublisher{
				URL:           aws.StringValue(lb.DNSName),
				TargetPort:    int(aws.Int64Value(tg.Port)),
				PublishedPort: int(aws.Int64Value(tg.Port)),
				Protocol:      aws.StringValue(tg.Protocol),
			})

		}
	}
	return loadBalancers, nil
}

func (s sdk) ListTasks(ctx context.Context, cluster string, family string) ([]string, error) {
	tasks, err := s.ECS.ListTasksWithContext(ctx, &ecs.ListTasksInput{
		Cluster: aws.String(cluster),
		Family:  aws.String(family),
	})
	if err != nil {
		return nil, err
	}
	arns := []string{}
	for _, arn := range tasks.TaskArns {
		arns = append(arns, *arn)
	}
	return arns, nil
}

func (s sdk) GetPublicIPs(ctx context.Context, interfaces ...string) (map[string]string, error) {
	desc, err := s.EC2.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: aws.StringSlice(interfaces),
	})
	if err != nil {
		return nil, err
	}
	publicIPs := map[string]string{}
	for _, interf := range desc.NetworkInterfaces {
		if interf.Association != nil {
			publicIPs[aws.StringValue(interf.NetworkInterfaceId)] = aws.StringValue(interf.Association.PublicIp)
		}
	}
	return publicIPs, nil
}

func (s sdk) LoadBalancerType(ctx context.Context, arn string) (string, error) {
	logrus.Debug("Check if LoadBalancer exists: ", arn)
	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return "", err
	}
	if len(lbs.LoadBalancers) == 0 {
		return "", fmt.Errorf("load balancer does not exist: %s", arn)
	}
	return aws.StringValue(lbs.LoadBalancers[0].Type), nil
}

func (s sdk) GetLoadBalancerURL(ctx context.Context, arn string) (string, error) {
	logrus.Debug("Retrieve load balancer URL: ", arn)
	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return "", err
	}
	dnsName := aws.StringValue(lbs.LoadBalancers[0].DNSName)
	if dnsName == "" {
		return "", fmt.Errorf("Load balancer %s doesn't have a dns name", aws.StringValue(lbs.LoadBalancers[0].LoadBalancerArn))
	}
	return dnsName, nil
}

func (s sdk) WithVolumeSecurityGroups(ctx context.Context, id string, fn func(securityGroups []string) error) error {
	mounts, err := s.EFS.DescribeMountTargetsWithContext(ctx, &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String(id),
	})
	if err != nil {
		return err
	}
	for _, mount := range mounts.MountTargets {
		groups, err := s.EFS.DescribeMountTargetSecurityGroupsWithContext(ctx, &efs.DescribeMountTargetSecurityGroupsInput{
			MountTargetId: mount.MountTargetId,
		})
		if err != nil {
			return err
		}
		err = fn(aws.StringValueSlice(groups.SecurityGroups))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s sdk) GetParameter(ctx context.Context, name string) (string, error) {
	parameter, err := s.SSM.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return "", err
	}

	value := *parameter.Parameter.Value
	var ami struct {
		SchemaVersion     int    `json:"schema_version"`
		ImageName         string `json:"image_name"`
		ImageID           string `json:"image_id"`
		OS                string `json:"os"`
		ECSRuntimeVersion string `json:"ecs_runtime_verion"`
		ECSAgentVersion   string `json:"ecs_agent_version"`
	}
	err = json.Unmarshal([]byte(value), &ami)
	if err != nil {
		return "", err
	}

	return ami.ImageID, nil
}
