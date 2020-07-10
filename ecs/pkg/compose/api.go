package compose

import (
	"context"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
)

type API interface {
	Up(ctx context.Context, options cli.ProjectOptions) error
	Down(ctx context.Context, options cli.ProjectOptions) error

	CreateContextData(ctx context.Context, params map[string]string) (contextData interface{}, description string, err error)

	Convert(project *types.Project) (*cloudformation.Template, error)
	Logs(ctx context.Context, projectName cli.ProjectOptions) error
	Ps(background context.Context, options cli.ProjectOptions) ([]ServiceStatus, error)

	CreateSecret(ctx context.Context, secret Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (Secret, error)
	ListSecrets(ctx context.Context) ([]Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}
