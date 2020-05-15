package run

import (
	"strconv"

	"github.com/docker/go-connections/nat"

	"github.com/docker/api/containers"
)

type runOpts struct {
	name    string
	publish []string
}

func toPorts(ports []string) ([]containers.Port, error) {
	_, bindings, err := nat.ParsePortSpecs(ports)
	if err != nil {
		return nil, err
	}
	var result []containers.Port

	for port, bind := range bindings {
		for _, portbind := range bind {
			var hostPort uint32
			if portbind.HostPort != "" {
				hp, err := strconv.Atoi(portbind.HostPort)
				if err != nil {
					return nil, err
				}
				hostPort = uint32(hp)
			} else {
				hostPort = uint32(port.Int())
			}

			result = append(result, containers.Port{
				HostPort:      hostPort,
				ContainerPort: uint32(port.Int()),
				Protocol:      port.Proto(),
				HostIP:        portbind.HostIP,
			})
		}
	}

	return result, nil
}

func (r *runOpts) toContainerConfig(image string) (containers.ContainerConfig, error) {
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
