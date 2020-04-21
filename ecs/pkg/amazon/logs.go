package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/sirupsen/logrus"
)

// GetOrCreateLogGroup retrieve a pre-existing log group for project or create one
func (c client) GetOrCreateLogGroup(project *compose.Project) (*string, error) {
	logrus.Debug("Create Log Group")
	logGroup := fmt.Sprintf("/ecs/%s", project.Name)
	_, err := c.CW.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroup),
		Tags: map[string]*string{
			ProjectTag: aws.String(project.Name),
		},
	})
	if err != nil {
		if _, ok := err.(*cloudwatchlogs.ResourceAlreadyExistsException); !ok {
			return nil, err
		}
	}
	return &logGroup, nil
}
