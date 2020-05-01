package cmd

import (
	"strconv"
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
		source, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return nil, err
		}
		destination, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return nil, err
		}

		result = append(result, containers.Port{
			Source:      uint32(source),
			Destination: uint32(destination),
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
