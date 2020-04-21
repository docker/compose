package amazon

import (
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
	"github.com/docker/ecs-plugin/pkg/compose"
)

const (
	ProjectTag = "com.docker.compose.project"
)

func NewClient(profile string, cluster string, region string) (compose.API, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: profile,
		Config: aws.Config{
			Region: aws.String(region),
		},
	})
	if err != nil {
		return nil, err
	}
	return &client{
		Cluster: cluster,
		Region:  region,
		sess:    sess,
		ECS:     ecs.New(sess),
		EC2:     ec2.New(sess),
		ELB:     elbv2.New(sess),
		CW:      cloudwatchlogs.New(sess),
		IAM:     iam.New(sess),
		CF:      cloudformation.New(sess),
	}, nil
}

type client struct {
	Cluster string
	Region  string
	sess    *session.Session
	ECS     ecsiface.ECSAPI
	EC2     ec2iface.EC2API
	ELB     elbv2iface.ELBV2API
	CW      cloudwatchlogsiface.CloudWatchLogsAPI
	IAM     iamiface.IAMAPI
	CF      cloudformationiface.CloudFormationAPI
}

var _ compose.API = &client{}
