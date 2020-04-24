package amazon

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/sirupsen/logrus"
)

func (c client) RegisterTaskDefinition(task *ecs.RegisterTaskDefinitionInput) (*string, error) {
	logrus.Debug("Register Task Definition")
	def, err := c.ECS.RegisterTaskDefinition(task)
	if err != nil {
		return nil, err
	}
	return def.TaskDefinition.TaskDefinitionArn, err
}

func (c client) CreateCluster() (*string, error) {
	logrus.Debug("Create cluster ", c.Cluster)
	response, err := c.ECS.CreateCluster(&ecs.CreateClusterInput{ClusterName: &c.Cluster})
	if err != nil {
		return nil, err
	}
	return response.Cluster.Status, nil
}

func (c client) DeleteCluster() error {
	logrus.Debug("Delete cluster ", c.Cluster)
	response, err := c.ECS.DeleteCluster(&ecs.DeleteClusterInput{Cluster: &c.Cluster})
	if err != nil {
		return err
	}
	if *response.Cluster.Status == "INACTIVE" {
		return nil
	}
	return errors.New("Failed to delete cluster, status: " + *response.Cluster.Status)
}

func (c client) ClusterExists() (bool, error) {
	logrus.Debug("Check if cluster was already created: ", c.Cluster)
	clusters, err := c.ECS.DescribeClusters(&ecs.DescribeClustersInput{
		Clusters: []*string{aws.String(c.Cluster)},
	})
	if err != nil {
		return false, err
	}
	return len(clusters.Clusters) > 0, nil
}
