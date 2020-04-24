package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/cloudformation"
	cf "github.com/aws/aws-sdk-go/service/cloudformation"
)

func (c *client) ComposeDown(projectName *string, keepLoadBalancer, deleteCluster bool) error {
	_, err := c.CF.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: projectName,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Delete stack ")
	if err = c.CF.WaitUntilStackDeleteComplete(&cf.DescribeStacksInput{StackName: projectName}); err != nil {
		return err
	}
	fmt.Printf("... done.\n")

	if !deleteCluster {
		return nil
	}

	fmt.Printf("Delete cluster %s", c.Cluster)
	if err = c.DeleteCluster(); err != nil {
		return err
	}
	fmt.Printf("... done. \n")
	return nil
}
