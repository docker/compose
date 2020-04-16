package amazon

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
)

const ECSTaskExecutionPolicy = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"

var defaultTaskExecutionRole *string

// GetEcsTaskExecutionRole retrieve the role ARN to apply for task execution
func (c client) GetEcsTaskExecutionRole(spec *types.ServiceConfig) (*string, error) {
	if arn, ok := spec.Extras["x-ecs-TaskExecutionRole"]; ok {
		s := arn.(string)
		return &s, nil
	}
	if defaultTaskExecutionRole != nil {
		return defaultTaskExecutionRole, nil
	}

	logrus.Debug("Retrieve Task Execution Role")
	entities, err := c.IAM.ListEntitiesForPolicy(&iam.ListEntitiesForPolicyInput{
		EntityFilter: aws.String("Role"),
		PolicyArn:    aws.String(ECSTaskExecutionPolicy),
	})
	if err != nil {
		return nil, err
	}
	if len(entities.PolicyRoles) == 0 {
		return nil, fmt.Errorf("no Role is attached to AmazonECSTaskExecutionRole Policy, please provide an explicit task execution role")
	}
	if len(entities.PolicyRoles) > 1 {
		return nil, fmt.Errorf("multiple Roles are attached to AmazonECSTaskExecutionRole Policy, please provide an explicit task execution role")
	}

	role, err := c.IAM.GetRole(&iam.GetRoleInput{
		RoleName: entities.PolicyRoles[0].RoleName,
	})
	if err != nil {
		return nil, err
	}
	defaultTaskExecutionRole = role.Role.Arn
	return role.Role.Arn, nil
}
