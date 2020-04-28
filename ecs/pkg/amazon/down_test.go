package amazon

import (
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

	recorder := m.EXPECT()
	recorder.DeleteStack("test_project").Return(nil).Times(1)

	c.ComposeDown("test_project", false)
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

	recorder := m.EXPECT()
	recorder.DeleteStack("test_project").Return(nil).Times(1)
	recorder.DeleteCluster("test_cluster").Return(nil).Times(1)

	c.ComposeDown("test_project", true)
}
