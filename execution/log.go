package execution

import (
	"context"

	"github.com/docker/containerd/log"
	"github.com/sirupsen/logrus"
)

var ctx context.Context

func GetLogger(module string) *logrus.Entry {
	if ctx == nil {
		ctx = log.WithModule(context.Background(), "execution")
	}

	subCtx := log.WithModule(ctx, module)
	return log.GetLogger(subCtx)
}
