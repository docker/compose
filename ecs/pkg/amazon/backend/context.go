package backend

import (
	"context"
	"fmt"
)

func (b *Backend) CreateContextData(ctx context.Context, params map[string]string) (contextData interface{}, description string, err error) {
	err = b.api.CheckRequirements(ctx)
	if err != nil {
		return "", "", err
	}

	if b.Cluster != "" {
		exists, err := b.api.ClusterExists(ctx, b.Cluster)
		if err != nil {
			return "", "", err
		}
		if !exists {
			return "", "", fmt.Errorf("cluster %s does not exists", b.Cluster)
		}
	}
	return "", "", nil
}
