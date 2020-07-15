package backend

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Ps(ctx context.Context, options cli.ProjectOptions) ([]compose.ServiceStatus, error) {
	project, err := cli.ProjectFromOptions(&options)
	if err != nil {
		return nil, err
	}

	resources, err := b.api.ListStackResources(ctx, project.Name)
	if err != nil {
		return nil, err
	}

	loadBalancer, err := b.GetLoadBalancer(ctx, project)
	if err != nil {
		return nil, err
	}

	cluster, err := b.GetCluster(ctx, project)
	if err != nil {
		return nil, err
	}

	servicesARN := []string{}
	for _, r := range resources {
		switch r.Type {
		case "AWS::ECS::Service":
			servicesARN = append(servicesARN, r.ARN)
		case "AWS::ECS::Cluster":
			cluster = r.ARN
		case "AWS::ElasticLoadBalancingV2::LoadBalancer":
			loadBalancer = r.ARN
		}
	}

	status, err := b.api.DescribeServices(ctx, cluster, servicesARN)
	if err != nil {
		return nil, err
	}

	url, err := b.api.GetLoadBalancerURL(ctx, loadBalancer)
	if err != nil {
		return nil, err
	}

	for i, state := range status {
		s, err := project.GetService(state.Name)
		if err != nil {
			return nil, err
		}
		ports := []string{}
		for _, p := range s.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", url, p.Published, p.Target, p.Protocol))
		}
		state.Ports = ports
		status[i] = state
	}
	return status, nil
}
