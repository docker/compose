package backend

import (
	"context"

	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b Backend) CreateSecret(ctx context.Context, secret compose.Secret) (string, error) {
	return b.api.CreateSecret(ctx, secret)
}

func (b Backend) InspectSecret(ctx context.Context, id string) (compose.Secret, error) {
	return b.api.InspectSecret(ctx, id)
}

func (b Backend) ListSecrets(ctx context.Context) ([]compose.Secret, error) {
	return b.api.ListSecrets(ctx)
}

func (b Backend) DeleteSecret(ctx context.Context, id string, recover bool) error {
	return b.api.DeleteSecret(ctx, id, recover)
}
