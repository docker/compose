package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
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
	}, nil
}

type client struct {
	Cluster string
	Region string
	sess *session.Session
}

var _ compose.API = &client{}
