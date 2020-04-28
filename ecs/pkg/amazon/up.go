package amazon

import (
	"fmt"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeUp(project *compose.Project) error {
	ok, err := c.api.ClusterExists(c.Cluster)
	if err != nil {
		return err
	}
	if !ok {
		c.api.CreateCluster(c.Cluster)
	}
	update, err := c.api.StackExists(project.Name)
	if err != nil {
		return err
	}
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := c.Convert(project)
	if err != nil {
		return err
	}

	err = c.api.CreateStack(project.Name, template)
	if err != nil {
		return err
	}

	err = c.api.DescribeStackEvents(project.Name)
	if err != nil {
		return err
	}

	// TODO monitor progress
	return nil
}

type upAPI interface {
	ClusterExists(name string) (bool, error)
	CreateCluster(name string) (string, error)
	StackExists(name string) (bool, error)
	CreateStack(name string, template *cloudformation.Template) error
	DescribeStackEvents(stack string) error
}
