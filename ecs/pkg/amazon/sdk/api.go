package sdk

import (
	"context"

	cf "github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

type API interface {
	GetDefaultVPC(ctx context.Context) (string, error)
	VpcExists(ctx context.Context, vpcID string) (bool, error)
	GetSubNets(ctx context.Context, vpcID string) ([]string, error)

	StackExists(ctx context.Context, name string) (bool, error)
	CreateStack(ctx context.Context, name string, template *cloudformation.Template, parameters map[string]string) error
	DeleteStack(ctx context.Context, name string) error
	ListStackResources(ctx context.Context, name string) ([]compose.StackResource, error)
	GetStackID(ctx context.Context, name string) (string, error)
	WaitStackComplete(ctx context.Context, name string, operation int) error
	DescribeStackEvents(ctx context.Context, stackID string) ([]*cf.StackEvent, error)

	DescribeServices(ctx context.Context, cluster string, arns []string) ([]compose.ServiceStatus, error)

	LoadBalancerExists(ctx context.Context, arn string) (bool, error)
	GetLoadBalancerURL(ctx context.Context, arn string) (string, error)

	ClusterExists(ctx context.Context, name string) (bool, error)

	GetLogs(ctx context.Context, name string, consumer compose.LogConsumer) error

	CreateSecret(ctx context.Context, secret compose.Secret) (string, error)
	InspectSecret(ctx context.Context, id string) (compose.Secret, error)
	ListSecrets(ctx context.Context) ([]compose.Secret, error)
	DeleteSecret(ctx context.Context, id string, recover bool) error
}
