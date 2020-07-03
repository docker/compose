package sdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/ecs-plugin/internal"

	"github.com/aws/aws-sdk-go/aws/request"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudformation/cloudformationiface"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	cf "github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/sirupsen/logrus"
)

type sdk struct {
	sess *session.Session
	ECS  ecsiface.ECSAPI
	EC2  ec2iface.EC2API
	ELB  elbv2iface.ELBV2API
	CW   cloudwatchlogsiface.CloudWatchLogsAPI
	IAM  iamiface.IAMAPI
	CF   cloudformationiface.CloudFormationAPI
	SM   secretsmanageriface.SecretsManagerAPI
}

func NewAPI(sess *session.Session) API {
	sess.Handlers.Build.PushBack(func(r *request.Request) {
		request.AddToUserAgent(r, fmt.Sprintf("Docker CLI %s", internal.Version))
	})
	return sdk{
		ECS: ecs.New(sess),
		EC2: ec2.New(sess),
		ELB: elbv2.New(sess),
		CW:  cloudwatchlogs.New(sess),
		IAM: iam.New(sess),
		CF:  cloudformation.New(sess),
		SM:  secretsmanager.New(sess),
	}
}

func (s sdk) ClusterExists(ctx context.Context, name string) (bool, error) {
	logrus.Debug("Check if cluster was already created: ", name)
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

func (s sdk) VpcExists(ctx context.Context, vpcID string) (bool, error) {
	logrus.Debug("Check if VPC exists: ", vpcID)
	_, err := s.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{VpcIds: []*string{&vpcID}})
	return err == nil, err
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
			{
				Name:   aws.String("default-for-az"),
				Values: []*string{aws.String("true")},
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
		// FIXME doesn't work as expected
		return false, nil
	}
	return len(stacks.Stacks) > 0, nil
}

func (s sdk) CreateStack(ctx context.Context, name string, template *cf.Template, parameters map[string]string) error {
	logrus.Debug("Create CloudFormation stack")
	json, err := template.JSON()
	if err != nil {
		return err
	}

	param := []*cloudformation.Parameter{}
	for name, value := range parameters {
		param = append(param, &cloudformation.Parameter{
			ParameterKey:     aws.String(name),
			ParameterValue:   aws.String(value),
			UsePreviousValue: aws.Bool(true),
		})
	}

	_, err = s.CF.CreateStackWithContext(ctx, &cloudformation.CreateStackInput{
		OnFailure:        aws.String("DELETE"),
		StackName:        aws.String(name),
		TemplateBody:     aws.String(string(json)),
		Parameters:       param,
		TimeoutInMinutes: aws.Int64(15),
		Capabilities: []*string{
			aws.String(cloudformation.CapabilityCapabilityIam),
		},
	})
	return err
}

func (s sdk) WaitStackComplete(ctx context.Context, name string, operation int) error {
	input := &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	}
	switch operation {
	case compose.StackCreate:
		return s.CF.WaitUntilStackCreateCompleteWithContext(ctx, input)
	case compose.StackDelete:
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

func (s sdk) ListStackResources(ctx context.Context, name string) ([]compose.StackResource, error) {
	// FIXME handle pagination
	res, err := s.CF.ListStackResourcesWithContext(ctx, &cloudformation.ListStackResourcesInput{
		StackName: aws.String(name),
	})
	if err != nil {
		return nil, err
	}

	resources := []compose.StackResource{}
	for _, r := range res.StackResourceSummaries {
		resources = append(resources, compose.StackResource{
			LogicalID: *r.LogicalResourceId,
			Type:      *r.ResourceType,
			ARN:       *r.PhysicalResourceId,
			Status:    *r.ResourceStatus,
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

func (s sdk) CreateSecret(ctx context.Context, secret compose.Secret) (string, error) {
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
	return *response.ARN, nil
}

func (s sdk) InspectSecret(ctx context.Context, id string) (compose.Secret, error) {
	logrus.Debug("Inspect secret " + id)
	response, err := s.SM.DescribeSecret(&secretsmanager.DescribeSecretInput{SecretId: &id})
	if err != nil {
		return compose.Secret{}, err
	}
	labels := map[string]string{}
	for _, tag := range response.Tags {
		labels[*tag.Key] = *tag.Value
	}
	secret := compose.Secret{
		ID:     *response.ARN,
		Name:   *response.Name,
		Labels: labels,
	}
	if response.Description != nil {
		secret.Description = *response.Description
	}
	return secret, nil
}

func (s sdk) ListSecrets(ctx context.Context) ([]compose.Secret, error) {
	logrus.Debug("List secrets ...")
	response, err := s.SM.ListSecrets(&secretsmanager.ListSecretsInput{})
	if err != nil {
		return []compose.Secret{}, err
	}
	var secrets []compose.Secret

	for _, sec := range response.SecretList {

		labels := map[string]string{}
		for _, tag := range sec.Tags {
			labels[*tag.Key] = *tag.Value
		}
		description := ""
		if sec.Description != nil {
			description = *sec.Description
		}
		secrets = append(secrets, compose.Secret{
			ID:          *sec.ARN,
			Name:        *sec.Name,
			Labels:      labels,
			Description: description,
		})
	}
	return secrets, nil
}

func (s sdk) DeleteSecret(ctx context.Context, id string, recover bool) error {
	logrus.Debug("List secrets ...")
	force := !recover
	_, err := s.SM.DeleteSecret(&secretsmanager.DeleteSecretInput{SecretId: &id, ForceDeleteWithoutRecovery: &force})
	return err
}

func (s sdk) GetLogs(ctx context.Context, name string, consumer compose.LogConsumer) error {
	logGroup := fmt.Sprintf("/docker-compose/%s", name)
	var startTime int64
	for {
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
				p := strings.Split(*event.LogStreamName, "/")
				consumer.Log(p[1], p[2], *event.Message)
				startTime = *event.IngestionTime
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
				name = *t.Value
			}
		}
		if name == "" {
			return nil, fmt.Errorf("service %s doesn't have a %s tag", *service.ServiceArn, compose.ServiceTag)
		}
		status = append(status, compose.ServiceStatus{
			ID:       *service.ServiceName,
			Name:     name,
			Replicas: int(*service.RunningCount),
			Desired:  int(*service.DesiredCount),
		})
	}

	return status, nil
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
			publicIPs[*interf.NetworkInterfaceId] = *interf.Association.PublicIp
		}
	}
	return publicIPs, nil
}

func (s sdk) LoadBalancerExists(ctx context.Context, arn string) (bool, error) {
	logrus.Debug("Check if LoadBalancer exists: ", arn)
	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return false, err
	}
	return len(lbs.LoadBalancers) > 0, nil
}

func (s sdk) GetLoadBalancerURL(ctx context.Context, arn string) (string, error) {
	logrus.Debug("Retrieve load balancer URL: ", arn)
	lbs, err := s.ELB.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(arn)},
	})
	if err != nil {
		return "", err
	}
	return *lbs.LoadBalancers[0].DNSName, nil
}
