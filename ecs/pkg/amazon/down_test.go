package amazon

import (
	"context"
	"testing"

	"github.com/docker/ecs-plugin/pkg/amazon/mock"
	"github.com/golang/mock/gomock"
)

func TestDownDontDeleteCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock.NewMockAPI(ctrl)
	c := &client{
		Cluster: "test_cluster",
		Region:  "region",
		api:     m,
	}
	ctx := context.TODO()
	recorder := m.EXPECT()
	recorder.DeleteStack(ctx, "test_project").Return(nil).Times(1)
	recorder.WaitStackComplete(ctx, "test_project", gomock.Any()).Return(nil).Times(1)

	c.ComposeDown(ctx, "test_project", false)
}

func TestDownDeleteCluster(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock.NewMockAPI(ctrl)
	c := &client{
		Cluster: "test_cluster",
		Region:  "region",
		api:     m,
	}

	ctx := context.TODO()
	recorder := m.EXPECT()
	recorder.DeleteStack(ctx, "test_project").Return(nil).Times(1)
	recorder.WaitStackComplete(ctx, "test_project", gomock.Any()).Return(nil).Times(1)
	recorder.DeleteCluster(ctx, "test_cluster").Return(nil).Times(1)

	c.ComposeDown(ctx, "test_project", true)
}
