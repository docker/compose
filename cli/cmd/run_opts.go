package cmd

import (
	"strings"

	"github.com/docker/api/containers"
)

type runOpts struct {
	name    string
	publish []string
}

func toPorts(ports []string) ([]containers.Port, error) {
	var result []containers.Port

	for _, port := range ports {
		parts := strings.Split(port, ":")
		result = append(result, containers.Port{
			Source:      parts[0],
			Destination: parts[1],
		})
	}
	return result, nil
}

func (r *runOpts) ToContainerConfig(image string) (containers.ContainerConfig, error) {
	publish, err := toPorts(r.publish)
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	return containers.ContainerConfig{
		ID:    r.name,
		Image: image,
		Ports: publish,
	}, nil
}
