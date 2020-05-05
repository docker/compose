package amazon

import (
	"context"

	"github.com/docker/ecs-plugin/pkg/docker"
)

type secretsAPI interface {
	CreateSecret(ctx context.Context, secret docker.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (docker.Secret, error)
	ListSecrets(ctx context.Context) ([]docker.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}

func (c client) CreateSecret(ctx context.Context, secret docker.Secret) (string, error) {
	return c.api.CreateSecret(ctx, secret)
}

func (c client) InspectSecret(ctx context.Context, id string) (docker.Secret, error) {
	return c.api.InspectSecret(ctx, id)
}

func (c client) ListSecrets(ctx context.Context) ([]docker.Secret, error) {
	return c.api.ListSecrets(ctx)
}

func (c client) DeleteSecret(ctx context.Context, id string, recover bool) error {
	return c.api.DeleteSecret(ctx, id, recover)
}
