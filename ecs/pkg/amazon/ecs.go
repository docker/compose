package amazon

import (
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
