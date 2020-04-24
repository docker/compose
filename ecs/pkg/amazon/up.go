package amazon

import (
	"fmt"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeUp(project *compose.Project, loadBalancerArn *string) error {
	ok, err := c.api.ClusterExists(c.Cluster)
	if err != nil {
		return err
	}
	if !ok {
		c.api.CreateCluster(c.Cluster)
	}
	update, err := c.api.StackExists(project.Name)
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := c.Convert(project, loadBalancerArn)
	if err != nil {
		return err
	}

	err = c.api.CreateStack(project.Name, template)
	if err != nil {
		return err
	}

	err = c.api.DescribeStackEvents(project.Name)

	// TODO monitor progress
	return nil
}
