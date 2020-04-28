package amazon

import (
	"context"
	"fmt"
)

func (c *client) ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error {
	err := c.api.DeleteStack(ctx, projectName)
	if err != nil {
		return err
	}
	fmt.Printf("Delete stack ")

	if !deleteCluster {
		return nil
	}

	fmt.Printf("Delete cluster %s", c.Cluster)
	if err = c.api.DeleteCluster(ctx, c.Cluster); err != nil {
		return err
	}
	fmt.Printf("... done. \n")
	return nil
}

type downAPI interface {
	DeleteStack(ctx context.Context, name string) error
	DeleteCluster(ctx context.Context, name string) error
}
