package amazon

import (
	"context"
)

func (c *client) ComposeLogs(ctx context.Context, projectName string) error {
	return c.api.GetLogs(ctx, projectName)
}

type logsAPI interface {
	GetLogs(ctx context.Context, name string) error
}
