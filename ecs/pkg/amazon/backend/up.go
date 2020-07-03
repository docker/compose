package backend

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
	"github.com/docker/ecs-plugin/pkg/console"
)

func (b *Backend) Up(ctx context.Context, options cli.ProjectOptions) error {
	project, err := cli.ProjectFromOptions(&options)
	if err != nil {
		return err
	}

	if b.Cluster != "" {
		ok, err := b.api.ClusterExists(ctx, b.Cluster)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("configured cluster %q does not exist", b.Cluster)
		}
	}

	update, err := b.api.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	if update {
		return fmt.Errorf("we do not (yet) support updating an existing CloudFormation stack")
	}

	template, err := b.Convert(project)
	if err != nil {
		return err
	}

	vpc, err := b.GetVPC(ctx, project)
	if err != nil {
		return err
	}

	subNets, err := b.api.GetSubNets(ctx, vpc)
	if err != nil {
		return err
	}

	lb, err := b.GetLoadBalancer(ctx, project)
	if err != nil {
		return err
	}

	parameters := map[string]string{
		ParameterClusterName:     b.Cluster,
		ParameterVPCId:           vpc,
		ParameterSubnet1Id:       subNets[0],
		ParameterSubnet2Id:       subNets[1],
		ParameterLoadBalancerARN: lb,
	}

	err = b.api.CreateStack(ctx, project.Name, template, parameters)
	if err != nil {
		return err
	}

	fmt.Println()
	w := console.NewProgressWriter()
	for k := range template.Resources {
		w.ResourceEvent(k, "PENDING", "")
	}
	return b.WaitStackCompletion(ctx, project.Name, compose.StackCreate, w)
}

func (b Backend) GetVPC(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if vpc, ok := project.Extensions[compose.ExtensionVPC]; ok {
		vpcID := vpc.(string)
		ok, err := b.api.VpcExists(ctx, vpcID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("VPC does not exist: %s", vpc)
		}
	}
	defaultVPC, err := b.api.GetDefaultVPC(ctx)
	if err != nil {
		return "", err
	}
	return defaultVPC, nil
}

func (b Backend) GetLoadBalancer(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if lb, ok := project.Extensions[compose.ExtensionLB]; ok {
		lbName := lb.(string)
		ok, err := b.api.LoadBalancerExists(ctx, lbName)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("Load Balancer does not exist: %s", lb)
		}
	}
	return "", nil
}
