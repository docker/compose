package amazon

import (
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
	cf "github.com/awslabs/goformation/v4/cloudformation"
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
}

func NewAPI(sess *session.Session) API {
	return sdk{
		ECS: ecs.New(sess),
		EC2: ec2.New(sess),
		ELB: elbv2.New(sess),
		CW:  cloudwatchlogs.New(sess),
		IAM: iam.New(sess),
		CF:  cloudformation.New(sess),
	}
}

func (s sdk) ClusterExists(name string) (bool, error) {
	logrus.Debug("Check if cluster was already created: ", name)
	clusters, err := s.ECS.DescribeClusters(&ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(name)},
	})
	if err != nil {
		return false, err
	}
	return len(clusters.Clusters) > 0, nil
}

func (s sdk) CreateCluster(name string) (string, error) {
	logrus.Debug("Create cluster ", name)
	response, err := s.ECS.CreateCluster(&ecs.CreateClusterInput{ClusterName: aws.String(name)})
	if err != nil {
		return "", err
	}
	return *response.Cluster.Status, nil
}

func (s sdk) DeleteCluster(name string) error {
	logrus.Debug("Delete cluster ", name)
	response, err := s.ECS.DeleteCluster(&ecs.DeleteClusterInput{Cluster: aws.String(name)})
	if err != nil {
		return err
	}
	if *response.Cluster.Status == "INACTIVE" {
		return nil
	}
	return fmt.Errorf("Failed to delete cluster, status: %s" + *response.Cluster.Status)
}

func (s sdk) GetDefaultVPC() (string, error) {
	logrus.Debug("Retrieve default VPC")
	vpcs, err := s.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
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

func (s sdk) GetSubNets(vpc string) ([]string, error) {
	logrus.Debug("Retrieve SubNets")
	subnets, err := s.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		DryRun: nil,
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpc)},
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

func (s sdk) ListRolesForPolicy(policy string) ([]string, error) {
	entities, err := s.IAM.ListEntitiesForPolicy(&iam.ListEntitiesForPolicyInput{
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

func (s sdk) GetRoleArn(name string) (string, error) {
	role, err := s.IAM.GetRole(&iam.GetRoleInput{
		RoleName: aws.String(name),
	})
	if err != nil {
		return "", err
	}
	return *role.Role.Arn, nil
}

func (s sdk) StackExists(name string) (bool, error) {
	stacks, err := s.CF.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		// FIXME doesn't work as expected
		return false, nil
	}
	return len(stacks.Stacks) > 0, nil
}

func (s sdk) CreateStack(name string, template *cf.Template) error {
	logrus.Debug("Create CloudFormation stack")
	json, err := template.JSON()
	if err != nil {
		return err
	}

	_, err = s.CF.CreateStack(&cloudformation.CreateStackInput{
		OnFailure:        aws.String("DELETE"),
		StackName:        aws.String(name),
		TemplateBody:     aws.String(string(json)),
		TimeoutInMinutes: aws.Int64(10),
	})
	return err
}

func (s sdk) DescribeStackEvents(name string) error {
	// Fixme implement Paginator on Events and return as a chan(events)
	_, err := s.CF.DescribeStackEvents(&cloudformation.DescribeStackEventsInput{
		StackName: aws.String(name),
	})
	return err
}

func (s sdk) DeleteStack(name string) error {
	logrus.Debug("Delete CloudFormation stack")
	_, err := s.CF.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: aws.String(name),
	})
	return err
}
