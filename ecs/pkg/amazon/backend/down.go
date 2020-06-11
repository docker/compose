package backend

import (
	"context"
	"fmt"

	"github.com/docker/ecs-plugin/pkg/amazon/types"
)

func (b *Backend) ComposeDown(ctx context.Context, projectName string, deleteCluster bool) error {
	err := b.api.DeleteStack(ctx, projectName)
	if err != nil {
		return err
	}

	err = b.WaitStackCompletion(ctx, projectName, types.StackDelete)
	if err != nil {
		return err
	}

	if !deleteCluster {
		return nil
	}

	fmt.Printf("Delete cluster %s", b.Cluster)
	if err = b.api.DeleteCluster(ctx, b.Cluster); err != nil {
		return err
	}
	fmt.Printf("... done. \n")
	return nil
}
