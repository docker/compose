package handrails

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type ServiceTopology struct {
	Name        string
	Ports       []string
	Volumes     []string
	Labels      map[string]string
	PodLabel    string
}

type Topology map[string]map[string]ServiceTopology // Project -> Service -> Topology

type TopologyMapper struct {
	client client.APIClient
}

func NewTopologyMapper(client client.APIClient) *TopologyMapper {
	return &TopologyMapper{client: client}
}

func (t *TopologyMapper) GetTopology(ctx context.Context) (Topology, error) {
	containers, err := t.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}

	topology := make(Topology)

	for _, c := range containers {
		project := c.Labels["com.docker.compose.project"]
		if project == "" {
			continue // Filter only compose containers
		}
		service := c.Labels["com.docker.compose.service"]
		podLabel := c.Labels["com.mcp.pod"]

		if topology[project] == nil {
			topology[project] = make(map[string]ServiceTopology)
		}

		var ports []string
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				ports = append(ports, string(p.PublicPort)) // Simplified string representation
			}
		}

		// volumes are in Mounts
		var volumes []string
		for _, m := range c.Mounts {
			volumes = append(volumes, m.Source)
		}

		topology[project][service] = ServiceTopology{
			Name:     c.Names[0],
			Ports:    ports,
			Volumes:  volumes,
			Labels:   c.Labels,
			PodLabel: podLabel,
		}
	}

	return topology, nil
}
