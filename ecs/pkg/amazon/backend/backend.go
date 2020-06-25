package backend

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/docker/ecs-plugin/pkg/amazon/sdk"
)

func NewBackend(profile string, cluster string, region string) (*Backend, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: profile,
		Config: aws.Config{
			Region: aws.String(region),
		},
	})
	if err != nil {
		return nil, err
	}
	return &Backend{
		Cluster: cluster,
		Region:  region,
		api:     sdk.NewAPI(sess),
	}, nil
}

type Backend struct {
	Cluster string
	Region  string
	api     sdk.API
}
