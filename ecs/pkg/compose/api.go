package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
)

type API interface {
	Convert(ctx context.Context, project *Project) (*cloudformation.Template, error)
	ComposeUp(ctx context.Context, project *Project) error
	ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error

	CreateSecret(ctx context.Context, name string, content string) (string, error)
	InspectSecret(ctx context.Context, name string) error
	ListSecrets(ctx context.Context) error
	DeleteSecret(ctx context.Context, name string) error
}
