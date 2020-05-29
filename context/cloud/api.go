package cloud

import (
	"context"

	"github.com/docker/api/errdefs"
)

// Service cloud specific services
type Service interface {
	// Login login to cloud provider
	Login(ctx context.Context, params map[string]string) error
}

// NotImplementedCloudService to use for backend that don't provide cloud services
func NotImplementedCloudService() (Service, error) {
	return notImplementedCloudService{}, nil
}

type notImplementedCloudService struct {
}

func (cs notImplementedCloudService) Login(ctx context.Context, params map[string]string) error {
	return errdefs.ErrNotImplemented
}
