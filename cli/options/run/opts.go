package run

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"

	"github.com/docker/api/containers"
)

// Opts contain run command options
type Opts struct {
	Name    string
	Publish []string
	Labels  []string
}

// ToContainerConfig convert run options to a container configuration
func (r *Opts) ToContainerConfig(image string) (containers.ContainerConfig, error) {
	publish, err := r.toPorts()
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	labels, err := toLabels(r.Labels)
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	return containers.ContainerConfig{
		ID:     r.Name,
		Image:  image,
		Ports:  publish,
		Labels: labels,
	}, nil
}

func (r *Opts) toPorts() ([]containers.Port, error) {
	_, bindings, err := nat.ParsePortSpecs(r.Publish)
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

func toLabels(labels []string) (map[string]string, error) {
	result := map[string]string{}
	for _, label := range labels {
		parts := strings.Split(label, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("wrong label format %q", label)
		}
		result[parts[0]] = parts[1]
	}

	return result, nil
}
