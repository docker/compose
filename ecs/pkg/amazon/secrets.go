package amazon

import (
	"context"
)

type secretsAPI interface {
	CreateSecret(ctx context.Context, name string, content string) (string, error)
	InspectSecret(ctx context.Context, name string) error
	ListSecrets(ctx context.Context) error
	DeleteSecret(ctx context.Context, name string) error
}

func (c client) CreateSecret(ctx context.Context, name string, content string) (string, error) {
	return c.api.CreateSecret(ctx, name, content)
}

func (c client) InspectSecret(ctx context.Context, name string) error {
	return c.api.InspectSecret(ctx, name)
}

func (c client) ListSecrets(ctx context.Context) error {
	return c.api.ListSecrets(ctx)
}

func (c client) DeleteSecret(ctx context.Context, name string) error {
	return c.api.DeleteSecret(ctx, name)
}
