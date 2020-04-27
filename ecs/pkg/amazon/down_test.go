package amazon

import (
	"github.com/docker/ecs-plugin/pkg/amazon/mock"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/golang/mock/gomock"
	"testing"
)

func Test_down_dont_delete_cluster(t *testing.T) {
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

	c.ComposeDown(&compose.Project{
		Name: "test_project",
	}, false, false)
}

func Test_down_delete_cluster(t *testing.T) {
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

	c.ComposeDown(&compose.Project{
		Name: "test_project",
	}, false, true)
}