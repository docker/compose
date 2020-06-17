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

	status := []compose.ServiceStatus{}
	for _, service := range project.Services {
		desc, err := b.api.DescribeService(ctx, cluster, service.Name)
		if err != nil {
			return nil, err
		}
		ports := []string{}
		for _, p := range service.Ports {
			ports = append(ports, fmt.Sprintf("*:%d->%d/%s", p.Published, p.Target, p.Protocol))
		}
		desc.Ports = ports
		status = append(status, desc)
	}
	return status, nil
}
