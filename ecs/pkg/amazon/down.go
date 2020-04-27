package amazon

import (
	"fmt"
)

func (c *client) ComposeDown(projectName *string, keepLoadBalancer, deleteCluster bool) error {
	err := c.api.DeleteStack(projectName)
	if err != nil {
		return err
	}
	fmt.Printf("Delete stack ")

	if !deleteCluster {
		return nil
	}

	fmt.Printf("Delete cluster %s", c.Cluster)
	if err = c.api.DeleteCluster(c.Cluster); err != nil {
		return err
	}
	fmt.Printf("... done. \n")
	return nil
}

type downAPI interface {
	DeleteStack(name string) error
	DeleteCluster(name string) error
}
