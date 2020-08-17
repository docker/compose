package ecs

import (
	"context"
)

func (b ecsAPIService) CreateSecret(ctx context.Context, secret Secret) (string, error) {
	return b.SDK.CreateSecret(ctx, secret)
}

func (b ecsAPIService) InspectSecret(ctx context.Context, id string) (Secret, error) {
	return b.SDK.InspectSecret(ctx, id)
}

func (b ecsAPIService) ListSecrets(ctx context.Context) ([]Secret, error) {
	return b.SDK.ListSecrets(ctx)
}

func (b ecsAPIService) DeleteSecret(ctx context.Context, id string, recover bool) error {
	return b.SDK.DeleteSecret(ctx, id, recover)
}
