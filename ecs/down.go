package ecs

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
)

func (b *ecsAPIService) Down(ctx context.Context, options *cli.ProjectOptions) error {
	name, err := b.projectName(options)
	if err != nil {
		return err
	}

	err = b.SDK.DeleteStack(ctx, name)
	if err != nil {
		return err
	}
	return b.WaitStackCompletion(ctx, name, StackDelete)
}

func (b *ecsAPIService) projectName(options *cli.ProjectOptions) (string, error) {
	name := options.Name
	if name == "" {
		project, err := cli.ProjectFromOptions(options)
		if err != nil {
			return "", err
		}
		name = project.Name
	}
	return name, nil
}
