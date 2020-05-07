package run

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/api/containers"
)

type runOpts struct {
	name    string
	publish []string
	volumes []string
}

func toPorts(ports []string) ([]containers.Port, error) {
	var result []containers.Port

	for _, port := range ports {
		parts := strings.Split(port, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("unable to parse ports %q", port)
		}
		source, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		destination, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}

		result = append(result, containers.Port{
			HostPort:      uint32(source),
			ContainerPort: uint32(destination),
		})
	}
	return result, nil
}

func (r *runOpts) toContainerConfig(image string) (containers.ContainerConfig, error) {
	publish, err := toPorts(r.publish)
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	return containers.ContainerConfig{
		ID:      r.name,
		Image:   image,
		Ports:   publish,
		Volumes: r.volumes,
	}, nil
}
