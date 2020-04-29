package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/docker"
)

type API interface {
	Convert(ctx context.Context, project *Project) (*cloudformation.Template, error)
	ComposeUp(ctx context.Context, project *Project) error
	ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error

	CreateSecret(ctx context.Context, name string, secret string) (string, error)
	InspectSecret(ctx context.Context, id string) (docker.Secret, error)
	ListSecrets(ctx context.Context) ([]docker.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}
