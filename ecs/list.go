package ecs

import (
	"context"
	"fmt"
	"github.com/docker/api/compose"
	"strings"

	"github.com/compose-spec/compose-go/cli"
)

func (b *ecsAPIService) Ps(ctx context.Context, options *cli.ProjectOptions) ([]compose.ServiceStatus, error) {
	projectName, err := b.projectName(options)
	if err != nil {
		return nil, err
	}
	parameters, err := b.SDK.ListStackParameters(ctx, projectName)
	if err != nil {
		return nil, err
	}
	cluster := parameters[ParameterClusterName]

	resources, err := b.SDK.ListStackResources(ctx, projectName)
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
		}
	}
	if len(servicesARN) == 0 {
		return nil, nil
	}
	status, err := b.SDK.DescribeServices(ctx, cluster, servicesARN)
	if err != nil {
		return nil, err
	}

	for i, state := range status {
		ports := []string{}
		for _, lb := range state.LoadBalancers {
			ports = append(ports, fmt.Sprintf(
				"%s:%d->%d/%s",
				lb.URL,
				lb.PublishedPort,
				lb.TargetPort,
				strings.ToLower(lb.Protocol)))
		}
		state.Ports = ports
		status[i] = state
	}
	return status, nil
}
