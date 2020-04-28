package amazon

import (
	"context"
	"fmt"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeUp(ctx context.Context, project *compose.Project) error {
	ok, err := c.api.ClusterExists(ctx, c.Cluster)
	if err != nil {
		return err
	}
	if !ok {
		c.api.CreateCluster(ctx, c.Cluster)
	}
	update, err := c.api.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := c.Convert(ctx, project)
	if err != nil {
		return err
	}

	err = c.api.CreateStack(ctx, project.Name, template)
	if err != nil {
		return err
	}

	err = c.api.DescribeStackEvents(ctx, project.Name)
	if err != nil {
		return err
	}

	// TODO monitor progress
	return nil
}

type upAPI interface {
	ClusterExists(ctx context.Context, name string) (bool, error)
	CreateCluster(ctx context.Context, name string) (string, error)
	StackExists(ctx context.Context, name string) (bool, error)
	CreateStack(ctx context.Context, name string, template *cloudformation.Template) error
	DescribeStackEvents(ctx context.Context, stack string) error
}
