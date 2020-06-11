package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/amazon/types"
)

type API interface {
	Convert(project *Project) (*cloudformation.Template, error)
	ComposeUp(ctx context.Context, project *Project) error
	ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error
	ComposeLogs(ctx context.Context, projectName string) error

	CreateSecret(ctx context.Context, secret types.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (types.Secret, error)
	ListSecrets(ctx context.Context) ([]types.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
	ComposePs(background context.Context, project *Project) ([]types.TaskStatus, error)
}
