package backend

import (
	"context"

	"github.com/docker/ecs-plugin/pkg/docker"
)

const (
	ContextParamRegion  = "region"
	ContextParamProfile = "profile"
)

func (b *Backend) CreateContextData(ctx context.Context, params map[string]string) (contextData interface{}, description string, err error) {
	err = b.api.CheckRequirements(ctx)
	if err != nil {
		return "", "", err
	}

	return docker.AwsContext{
		Profile: params[ContextParamProfile],
		Region:  params[ContextParamRegion],
	}, "Amazon ECS context", nil
}
