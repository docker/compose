package backend

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	err = b.api.CheckRequirements(ctx, b.Region)
	if err != nil {
		return err
	}

	cluster, err := b.GetCluster(ctx, project)
	if err != nil {
		return err
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
	if len(subNets) < 2 {
		return fmt.Errorf("VPC %s should have at least 2 associated subnets in different availability zones", vpc)
	}

	lb, err := b.GetLoadBalancer(ctx, project)
	if err != nil {
		return err
	}

	parameters := map[string]string{
		ParameterClusterName:     cluster,
		ParameterVPCId:           vpc,
		ParameterSubnet1Id:       subNets[0],
		ParameterSubnet2Id:       subNets[1],
		ParameterLoadBalancerARN: lb,
	}

	update, err := b.api.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	operation := compose.StackCreate
	if update {
		operation = compose.StackUpdate
		changeset, err := b.api.CreateChangeSet(ctx, project.Name, template, parameters)
		if err != nil {
			return err
		}
		err = b.api.UpdateStack(ctx, changeset)
		if err != nil {
			return err
		}
	} else {
		err = b.api.CreateStack(ctx, project.Name, template, parameters)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	w := console.NewProgressWriter()
	for k := range template.Resources {
		w.ResourceEvent(k, "PENDING", "")
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("user interrupted deployment. Deleting stack...")
		b.Down(ctx, options)
	}()

	return b.WaitStackCompletion(ctx, project.Name, operation, w)
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
		return vpcID, nil
	}
	defaultVPC, err := b.api.GetDefaultVPC(ctx)
	if err != nil {
		return "", err
	}
	return defaultVPC, nil
}

func (b Backend) GetLoadBalancer(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if ext, ok := project.Extensions[compose.ExtensionLB]; ok {
		lb := ext.(string)
		ok, err := b.api.LoadBalancerExists(ctx, lb)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("load balancer does not exist: %s", lb)
		}
		return lb, nil
	}
	return "", nil
}

func (b Backend) GetCluster(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if ext, ok := project.Extensions[compose.ExtensionCluster]; ok {
		cluster := ext.(string)
		ok, err := b.api.ClusterExists(ctx, cluster)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("cluster does not exist: %s", cluster)
		}
		return cluster, nil
	}
	return "", nil
}
