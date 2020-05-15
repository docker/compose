package cloud

import "context"

// Service cloud specific services
type Service interface {
	// Login login to cloud provider
	Login(ctx context.Context, params map[string]string) error
}
