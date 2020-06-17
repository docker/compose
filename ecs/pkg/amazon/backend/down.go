package backend

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Down(ctx context.Context, options cli.ProjectOptions) error {
	name := options.Name
	if name == "" {
		project, err := cli.ProjectFromOptions(&options)
		if err != nil {
			return err
		}
		name = project.Name
	}

	err := b.api.DeleteStack(ctx, name)
	if err != nil {
		return err
	}

	err = b.WaitStackCompletion(ctx, name, compose.StackDelete)
	if err != nil {
		return err
	}
	return nil
}
