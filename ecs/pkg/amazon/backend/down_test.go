package backend

import (
	"context"
	"testing"

	"github.com/docker/ecs-plugin/pkg/amazon/sdk"
	btypes "github.com/docker/ecs-plugin/pkg/amazon/types"
	"github.com/golang/mock/gomock"
)

func TestDownDontDeleteCluster(t *testing.T) {
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
	recorder.WaitStackComplete(ctx, "stack-123", btypes.StackDelete).Return(nil)
	recorder.DescribeStackEvents(ctx, "stack-123").Return(nil, nil)

	c.ComposeDown(ctx, "test_project", false)
}

func TestDownDeleteCluster(t *testing.T) {
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
	recorder.WaitStackComplete(ctx, "stack-123", btypes.StackDelete).Return(nil)
	recorder.DescribeStackEvents(ctx, "stack-123").Return(nil, nil)
	recorder.DeleteCluster(ctx, "test_cluster").Return(nil)

	c.ComposeDown(ctx, "test_project", true)
}
