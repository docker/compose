package amazon

import (
	"github.com/awslabs/goformation/v4/cloudformation"
)

type API interface {
	ClusterExists(name string) (bool, error)
	CreateCluster(name string) (string, error)
	DeleteCluster(name string) error

	GetDefaultVPC() (string, error)
	GetSubNets(vpcId string) ([]string, error)

	ListRolesForPolicy(policy string) ([]string, error)
	GetRoleArn(name string) (string, error)

	StackExists(name string) (bool, error)
	CreateStack(name string, template *cloudformation.Template) error
	DescribeStackEvents(stack string) error
	DeleteStack(name string) error
}
