package backend

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/ecs-plugin/pkg/compose"
)

var targetGroupLogicalName = regexp.MustCompile("(.*)(TCP|UDP)([0-9]+)TargetGroup")

func (b *Backend) Ps(ctx context.Context, options cli.ProjectOptions) ([]compose.ServiceStatus, error) {
	projectName, err := b.projectName(options)
	if err != nil {
		return nil, err
	}
	parameters, err := b.api.ListStackParameters(ctx, projectName)
	if err != nil {
		return nil, err
	}
	loadBalancer := parameters[ParameterLoadBalancerARN]
	cluster := parameters[ParameterClusterName]

	resources, err := b.api.ListStackResources(ctx, projectName)
	if err != nil {
		return nil, err
	}

	servicesARN := []string{}
	targetGroups := []string{}
	for _, r := range resources {
		switch r.Type {
		case "AWS::ECS::Service":
			servicesARN = append(servicesARN, r.ARN)
		case "AWS::ECS::Cluster":
			cluster = r.ARN
		case "AWS::ElasticLoadBalancingV2::LoadBalancer":
			loadBalancer = r.ARN
		case "AWS::ElasticLoadBalancingV2::TargetGroup":
			targetGroups = append(targetGroups, r.LogicalID)
		}
	}

	if len(servicesARN) == 0 {
		return nil, nil
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
		ports := []string{}
		for _, tg := range targetGroups {
			groups := targetGroupLogicalName.FindStringSubmatch(tg)
			if groups[0] == state.Name {
				ports = append(ports, fmt.Sprintf("%s:%s->%s/%s", url, groups[2], groups[2], strings.ToLower(groups[1])))
			}
		}
		state.Ports = ports
		status[i] = state
	}
	return status, nil
}
