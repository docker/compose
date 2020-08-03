package backend

import (
	"context"
	"fmt"

	"github.com/docker/ecs-plugin/pkg/docker"
)

const (
	ContextParamRegion  = "region"
	ContextParamProfile = "profile"
)

func (b *Backend) CreateContextData(ctx context.Context, params map[string]string) (contextData interface{}, description string, err error) {
	region, ok := params[ContextParamRegion]
	if !ok {
		return nil, "", fmt.Errorf("%q parameter is required", ContextParamRegion)
	}
	profile, ok := params[ContextParamProfile]
	if !ok {
		return nil, "", fmt.Errorf("%q parameter is required", ContextParamProfile)
	}
	err = b.api.CheckRequirements(ctx, region)
	if err != nil {
		return "", "", err
	}

	return docker.AwsContext{
		Profile: profile,
		Region:  region,
	}, "Amazon ECS context", nil
}
