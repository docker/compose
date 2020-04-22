package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/cloudformation"
	cf "github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeDown(project *compose.Project, keepLoadBalancer bool) error {
	_, err := c.CF.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: &project.Name,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Delete stack ")
	if err = c.CF.WaitUntilStackDeleteComplete(&cf.DescribeStacksInput{StackName: &project.Name}); err != nil {
		return err
	}
	fmt.Printf("... done.\nDelete cluster %s", c.Cluster)
	if err = c.DeleteCluster(); err != nil {
		return err
	}
	fmt.Printf("... done. \n")
	return nil
}
