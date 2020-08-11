package backend

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/docker/ecs-plugin/pkg/amazon/sdk"
	"github.com/docker/ecs-plugin/pkg/progress"
)

func NewBackend(profile string, region string) (*Backend, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile:           profile,
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			Region: aws.String(region),
		},
	})
	if err != nil {
		return nil, err
	}

	return &Backend{
		Region: region,
		api:    sdk.NewAPI(sess),
	}, nil
}

type Backend struct {
	Region string
	api    sdk.API
	writer progress.Writer
}

func (b *Backend) SetWriter(context context.Context) {
	b.writer = progress.ContextWriter(context)
}
