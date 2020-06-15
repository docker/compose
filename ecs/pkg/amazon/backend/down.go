package backend

import (
	"context"

	"github.com/docker/ecs-plugin/pkg/amazon/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Down(ctx context.Context, options compose.ProjectOptions) error {
	name := options.Name
	if name == "" {
		project, err := compose.ProjectFromOptions(&options)
		if err != nil {
			return err
		}
		name = project.Name
	}

	err := b.api.DeleteStack(ctx, name)
	if err != nil {
		return err
	}

	err = b.WaitStackCompletion(ctx, name, types.StackDelete)
	if err != nil {
		return err
	}
	return nil
}
