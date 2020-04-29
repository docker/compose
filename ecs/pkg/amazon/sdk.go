package amazon

import (
	"context"
	"fmt"

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
	"github.com/sirupsen/logrus"

	"github.com/docker/ecs-plugin/pkg/docker"
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
	clusters, err := s.ECS.DescribeClustersWithContext(aws.Context(ctx), &ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(name)},
	})
	if err != nil {
		return false, err
	}
	return len(clusters.Clusters) > 0, nil
}

func (s sdk) CreateCluster(ctx context.Context, name string) (string, error) {
	logrus.Debug("Create cluster ", name)
	response, err := s.ECS.CreateClusterWithContext(aws.Context(ctx), &ecs.CreateClusterInput{ClusterName: aws.String(name)})
	if err != nil {
		return "", err
	}
	return *response.Cluster.Status, nil
}

func (s sdk) DeleteCluster(ctx context.Context, name string) error {
	logrus.Debug("Delete cluster ", name)
	response, err := s.ECS.DeleteClusterWithContext(aws.Context(ctx), &ecs.DeleteClusterInput{Cluster: aws.String(name)})
	if err != nil {
		return err
	}
	if *response.Cluster.Status == "INACTIVE" {
		return nil
	}
	return fmt.Errorf("Failed to delete cluster, status: %s" + *response.Cluster.Status)
}

func (s sdk) VpcExists(ctx context.Context, vpcID string) (bool, error) {
	logrus.Debug("Check if VPC exists: ", vpcID)
	_, err := s.EC2.DescribeVpcsWithContext(aws.Context(ctx), &ec2.DescribeVpcsInput{VpcIds: []*string{&vpcID}})
	return err == nil, err
}

func (s sdk) GetDefaultVPC(ctx context.Context) (string, error) {
	logrus.Debug("Retrieve default VPC")
	vpcs, err := s.EC2.DescribeVpcsWithContext(aws.Context(ctx), &ec2.DescribeVpcsInput{
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
	subnets, err := s.EC2.DescribeSubnetsWithContext(aws.Context(ctx), &ec2.DescribeSubnetsInput{
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

func (s sdk) ListRolesForPolicy(ctx context.Context, policy string) ([]string, error) {
	entities, err := s.IAM.ListEntitiesForPolicyWithContext(aws.Context(ctx), &iam.ListEntitiesForPolicyInput{
		EntityFilter: aws.String("Role"),
		PolicyArn:    aws.String(policy),
	})
	if err != nil {
		return nil, err
	}
	roles := []string{}
	for _, e := range entities.PolicyRoles {
		roles = append(roles, *e.RoleName)
	}
	return roles, nil
}

func (s sdk) GetRoleArn(ctx context.Context, name string) (string, error) {
	role, err := s.IAM.GetRoleWithContext(aws.Context(ctx), &iam.GetRoleInput{
		RoleName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *role.Role.Arn, nil
}

func (s sdk) StackExists(ctx context.Context, name string) (bool, error) {
	stacks, err := s.CF.DescribeStacksWithContext(aws.Context(ctx), &cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		// FIXME doesn't work as expected
		return false, nil
	}
	return len(stacks.Stacks) > 0, nil
}

func (s sdk) CreateStack(ctx context.Context, name string, template *cf.Template) error {
	logrus.Debug("Create CloudFormation stack")
	json, err := template.JSON()
	if err != nil {
		return err
	}

	_, err = s.CF.CreateStackWithContext(aws.Context(ctx), &cloudformation.CreateStackInput{
		OnFailure:        aws.String("DELETE"),
		StackName:        aws.String(name),
		TemplateBody:     aws.String(string(json)),
		TimeoutInMinutes: aws.Int64(10),
	})
	return err
}

func (s sdk) DescribeStackEvents(ctx context.Context, name string) error {
	// Fixme implement Paginator on Events and return as a chan(events)
	_, err := s.CF.DescribeStackEventsWithContext(aws.Context(ctx), &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(name),
	})
	return err
}

func (s sdk) DeleteStack(ctx context.Context, name string) error {
	logrus.Debug("Delete CloudFormation stack")
	_, err := s.CF.DeleteStackWithContext(aws.Context(ctx), &cloudformation.DeleteStackInput{
		StackName: aws.String(name),
	})
	return err
}

func (s sdk) CreateSecret(ctx context.Context, name string, secret string) (string, error) {
	logrus.Debug("Create secret " + name)
	response, err := s.SM.CreateSecret(&secretsmanager.CreateSecretInput{Name: &name, SecretString: &secret})
	if err != nil {
		return "", err
	}
	return *response.ARN, nil
}

func (s sdk) InspectSecret(ctx context.Context, id string) (docker.Secret, error) {
	logrus.Debug("Inspect secret " + id)
	response, err := s.SM.DescribeSecret(&secretsmanager.DescribeSecretInput{SecretId: &id})
	if err != nil {
		return docker.Secret{}, err
	}
	labels := map[string]string{}
	for _, tag := range response.Tags {
		labels[*tag.Key] = *tag.Value
	}
	secret := docker.Secret{
		ID:     *response.ARN,
		Name:   *response.Name,
		Labels: labels,
	}
	if response.Description != nil {
		secret.Description = *response.Description
	}
	return secret, nil
}

func (s sdk) ListSecrets(ctx context.Context) ([]docker.Secret, error) {

	logrus.Debug("List secrets ...")
	response, err := s.SM.ListSecrets(&secretsmanager.ListSecretsInput{})
	if err != nil {
		return []docker.Secret{}, err
	}
	var secrets []docker.Secret

	for _, sec := range response.SecretList {

		labels := map[string]string{}
		for _, tag := range sec.Tags {
			labels[*tag.Key] = *tag.Value
		}
		description := ""
		if sec.Description != nil {
			description = *sec.Description
		}
		secrets = append(secrets, docker.Secret{
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
