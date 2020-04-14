package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
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
		Region: region,
		sess: sess,
		ECS: ecs.New(sess),
		EC2:  ec2.New(sess),
		CW: cloudwatchlogs.New(sess),
		IAM: iam.New(sess),
	}, nil
}

type client struct {
	Cluster string
	Region string
	sess *session.Session
	ECS *ecs.ECS
	EC2 *ec2.EC2
	CW *cloudwatchlogs.CloudWatchLogs
	IAM *iam.IAM
}

var _ compose.API = &client{}
