package backend

import (
	"context"
	"testing"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/ecs-plugin/pkg/amazon/sdk"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/golang/mock/gomock"
)

func TestDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := sdk.NewMockAPI(ctrl)
	c := &Backend{
		Cluster: "test_cluster",
		Region:  "region",
		api:     m,
	}
	ctx := context.TODO()
	recorder := m.EXPECT()
	recorder.DeleteStack(ctx, "test_project").Return(nil)
	recorder.GetStackID(ctx, "test_project").Return("stack-123", nil)
	recorder.WaitStackComplete(ctx, "stack-123", compose.StackDelete).Return(nil)
	recorder.DescribeStackEvents(ctx, "stack-123").Return(nil, nil)

	c.Down(ctx, cli.ProjectOptions{
		ConfigPaths: []string{},
		Name:        "test_project",
	})
}
