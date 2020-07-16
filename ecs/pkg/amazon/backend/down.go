package backend

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/console"
)

func (b *Backend) Down(ctx context.Context, options cli.ProjectOptions) error {
	name, err := b.projectName(options)
	if err != nil {
		return err
	}

	err = b.api.DeleteStack(ctx, name)
	if err != nil {
		return err
	}

	w := console.NewProgressWriter()
	err = b.WaitStackCompletion(ctx, name, compose.StackDelete, w)
	if err != nil {
		return err
	}
	return nil
}

func (b *Backend) projectName(options cli.ProjectOptions) (string, error) {
	name := options.Name
	if name == "" {
		project, err := cli.ProjectFromOptions(&options)
		if err != nil {
			return "", err
		}
		name = project.Name
	}
	return name, nil
}
