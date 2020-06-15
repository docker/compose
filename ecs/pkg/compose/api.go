package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/amazon/types"
)

type API interface {
	Up(ctx context.Context, options ProjectOptions) error
	Down(ctx context.Context, options ProjectOptions) error

	Convert(project *Project) (*cloudformation.Template, error)
	Logs(ctx context.Context, projectName string) error
	Ps(background context.Context, project *Project) ([]types.TaskStatus, error)

	CreateSecret(ctx context.Context, secret types.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (types.Secret, error)
	ListSecrets(ctx context.Context) ([]types.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}
