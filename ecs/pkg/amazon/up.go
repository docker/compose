package amazon

import (
	"context"
	"fmt"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (c *client) ComposeUp(ctx context.Context, project *compose.Project) error {
	if c.Cluster != "" {
		ok, err := c.api.ClusterExists(ctx, c.Cluster)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("configured cluster %q does not exist", c.Cluster)
		}
	}

	update, err := c.api.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := c.Convert(project)
	if err != nil {
		return err
	}

	vpc, err := c.GetVPC(ctx, project)
	if err != nil {
		return err
	}

	subNets, err := c.api.GetSubNets(ctx, vpc)
	if err != nil {
		return err
	}

	lb, err := c.GetLoadBalancer(ctx, project)
	if err != nil {
		return err
	}

	parameters := map[string]string{
		ParameterClusterName:     c.Cluster,
		ParameterVPCId:           vpc,
		ParameterSubnet1Id:       subNets[0],
		ParameterSubnet2Id:       subNets[1],
		ParameterLoadBalancerARN: lb,
	}

	err = c.api.CreateStack(ctx, project.Name, template, parameters)
	if err != nil {
		return err
	}

	fmt.Println()
	return c.WaitStackCompletion(ctx, project.Name, StackCreate)
}

func (c client) GetVPC(ctx context.Context, project *compose.Project) (string, error) {
	//check compose file for custom VPC selected
	if vpc, ok := project.Extras[ExtensionVPC]; ok {
		vpcID := vpc.(string)
		ok, err := c.api.VpcExists(ctx, vpcID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("VPC does not exist: %s", vpc)
		}
	}
	defaultVPC, err := c.api.GetDefaultVPC(ctx)
	if err != nil {
		return "", err
	}
	return defaultVPC, nil
}

func (c client) GetLoadBalancer(ctx context.Context, project *compose.Project) (string, error) {
	//check compose file for custom VPC selected
	if lb, ok := project.Extras[ExtensionLB]; ok {
		lbName := lb.(string)
		ok, err := c.api.LoadBalancerExists(ctx, lbName)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("Load Balancer does not exist: %s", lb)
		}
		return c.api.GetLoadBalancerARN(ctx, lbName)
	}
	return "", nil
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
