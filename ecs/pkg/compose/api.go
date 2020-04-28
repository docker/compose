package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
)

type API interface {
	Convert(ctx context.Context, project *Project) (*cloudformation.Template, error)
	ComposeUp(ctx context.Context, project *Project) error
	ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error
}
