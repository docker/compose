package backend

import (
	"context"
	"fmt"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/ecs-plugin/pkg/compose"
)

func (b *Backend) Ps(ctx context.Context, project *types.Project) ([]compose.ServiceStatus, error) {
	cluster := b.Cluster
	if cluster == "" {
		cluster = project.Name
	}

	resources, err := b.api.ListStackResources(ctx, project.Name)
	if err != nil {
		return nil, err
	}

	var loadBalancer string
	if lb, ok := project.Extensions[compose.ExtensionLB]; ok {
		loadBalancer = lb.(string)
	}
	servicesARN := []string{}
	for _, r := range resources {
		switch r.Type {
		case "AWS::ECS::Service":
			servicesARN = append(servicesARN, r.ARN)
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
