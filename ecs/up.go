package ecs

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) Up(ctx context.Context, options *cli.ProjectOptions) error {
	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return err
	}

	err = b.SDK.CheckRequirements(ctx, b.Region)
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

	subNets, err := b.SDK.GetSubNets(ctx, vpc)
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

	update, err := b.SDK.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	operation := StackCreate
	if update {
		operation = StackUpdate
		changeset, err := b.SDK.CreateChangeSet(ctx, project.Name, template, parameters)
		if err != nil {
			return err
		}
		err = b.SDK.UpdateStack(ctx, changeset)
		if err != nil {
			return err
		}
	} else {
		err = b.SDK.CreateStack(ctx, project.Name, template, parameters)
		if err != nil {
			return err
		}
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("user interrupted deployment. Deleting stack...")
		b.Down(ctx, options)
	}()

	err = b.WaitStackCompletion(ctx, project.Name, operation)
	return err
}

func (b ecsAPIService) GetVPC(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if vpc, ok := project.Extensions[ExtensionVPC]; ok {
		vpcID := vpc.(string)
		ok, err := b.SDK.VpcExists(ctx, vpcID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("VPC does not exist: %s", vpc)
		}
		return vpcID, nil
	}
	defaultVPC, err := b.SDK.GetDefaultVPC(ctx)
	if err != nil {
		return "", err
	}
	return defaultVPC, nil
}

func (b ecsAPIService) GetLoadBalancer(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if ext, ok := project.Extensions[ExtensionLB]; ok {
		lb := ext.(string)
		ok, err := b.SDK.LoadBalancerExists(ctx, lb)
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

func (b ecsAPIService) GetCluster(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if ext, ok := project.Extensions[ExtensionCluster]; ok {
		cluster := ext.(string)
		ok, err := b.SDK.ClusterExists(ctx, cluster)
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
