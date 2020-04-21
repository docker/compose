package amazon

import (
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeDown(project *compose.Project, keepLoadBalancer bool) error {
	_, err := c.CF.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: &project.Name,
	})
	if err != nil {
		return err
	}

	// TODO monitor progress
	return nil
}
