/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package ecs

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/compose-spec/compose-go/types"
)

func (b *ecsAPIService) Up(ctx context.Context, project *types.Project) error {
	err := b.SDK.CheckRequirements(ctx, b.Region)
	if err != nil {
		return err
	}

	cluster, err := b.GetCluster(ctx, project)
	if err != nil {
		return err
	}

	template, err := b.Convert(ctx, project)
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
		parameterClusterName:     cluster,
		parameterVPCId:           vpc,
		parameterSubnet1Id:       subNets[0],
		parameterSubnet2Id:       subNets[1],
		parameterLoadBalancerARN: lb,
	}

	update, err := b.SDK.StackExists(ctx, project.Name)
	if err != nil {
		return err
	}
	operation := stackCreate
	if update {
		operation = stackUpdate
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
		b.Down(ctx, project.Name) // nolint:errcheck
	}()

	err = b.WaitStackCompletion(ctx, project.Name, operation)
	return err
}

func (b ecsAPIService) GetVPC(ctx context.Context, project *types.Project) (string, error) {
	var vpcID string
	//check compose file for custom VPC selected
	if vpc, ok := project.Extensions[extensionVPC]; ok {
		vpcID = vpc.(string)
	} else {
		defaultVPC, err := b.SDK.GetDefaultVPC(ctx)
		if err != nil {
			return "", err
		}
		vpcID = defaultVPC
	}

	err := b.SDK.CheckVPC(ctx, vpcID)
	if err != nil {
		return "", err
	}
	return vpcID, nil
}

func (b ecsAPIService) GetLoadBalancer(ctx context.Context, project *types.Project) (string, error) {
	//check compose file for custom VPC selected
	if ext, ok := project.Extensions[extensionLB]; ok {
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
	if ext, ok := project.Extensions[extensionCluster]; ok {
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
