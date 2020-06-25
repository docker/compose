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

	status, err := b.api.DescribeServices(ctx, cluster, project.Name)
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
			ports = append(ports, fmt.Sprintf("*:%d->%d/%s", p.Published, p.Target, p.Protocol))
		}
		state.Ports = ports
		status[i] = state
	}
	return status, nil
}
