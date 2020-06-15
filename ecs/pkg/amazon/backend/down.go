package backend

import (
	"context"

	"github.com/docker/ecs-plugin/pkg/amazon/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Down(ctx context.Context, options compose.ProjectOptions) error {
	project, err := compose.ProjectFromOptions(&options)
	if err != nil {
		return err
	}

	err = b.api.DeleteStack(ctx, project.Name)
	if err != nil {
		return err
	}

	err = b.WaitStackCompletion(ctx, project.Name, types.StackDelete)
	if err != nil {
		return err
	}
	return nil
}
