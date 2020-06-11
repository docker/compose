package sdk

import (
	"context"

	cf "github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/amazon/types"
)

//go:generate mockgen -destination=./api_mock.go -self_package "github.com/docker/ecs-plugin/pkg/amazon" -package=amazon . API

type API interface {
	downAPI
	upAPI
	logsAPI
	secretsAPI
	listAPI
}

type upAPI interface {
	waitAPI
	GetDefaultVPC(ctx context.Context) (string, error)
	VpcExists(ctx context.Context, vpcID string) (bool, error)
	GetSubNets(ctx context.Context, vpcID string) ([]string, error)

	ClusterExists(ctx context.Context, name string) (bool, error)
	StackExists(ctx context.Context, name string) (bool, error)
	CreateStack(ctx context.Context, name string, template *cloudformation.Template, parameters map[string]string) error

	LoadBalancerExists(ctx context.Context, name string) (bool, error)
	GetLoadBalancerARN(ctx context.Context, name string) (string, error)
}

type downAPI interface {
	DeleteStack(ctx context.Context, name string) error
	DeleteCluster(ctx context.Context, name string) error
}

type logsAPI interface {
	GetLogs(ctx context.Context, name string, consumer types.LogConsumer) error
}

type secretsAPI interface {
	CreateSecret(ctx context.Context, secret types.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (types.Secret, error)
	ListSecrets(ctx context.Context) ([]types.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}

type listAPI interface {
	ListTasks(ctx context.Context, cluster string, name string) ([]string, error)
	DescribeTasks(ctx context.Context, cluster string, arns ...string) ([]types.TaskStatus, error)
	GetPublicIPs(ctx context.Context, interfaces ...string) (map[string]string, error)
}

type waitAPI interface {
	GetStackID(ctx context.Context, name string) (string, error)
	WaitStackComplete(ctx context.Context, name string, operation int) error
	DescribeStackEvents(ctx context.Context, stackID string) ([]*cf.StackEvent, error)
}
