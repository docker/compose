package formatter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/api/containers"
)

type portGroup struct {
	first uint32
	last  uint32
}

// PortsString returns a human readable published ports
func PortsString(ports []containers.Port) string {
	groupMap := make(map[string]*portGroup)
	var (
		result       []string
		hostMappings []string
		groupMapKeys []string
	)

	sort.Slice(ports, func(i int, j int) bool {
		return comparePorts(ports[i], ports[j])
	})

	for _, port := range ports {
		// Simple case: HOST_IP:PORT1:PORT2
		hostIP := "0.0.0.0"
		if port.HostIP != "" {
			hostIP = port.HostIP
		}

		if port.HostPort != port.ContainerPort {
			hostMappings = append(hostMappings, fmt.Sprintf("%s:%d->%d/%s", hostIP, port.HostPort, port.ContainerPort, port.Protocol))
			continue
		}

		current := port.ContainerPort
		portKey := fmt.Sprintf("%s/%s", hostIP, port.Protocol)
		group := groupMap[portKey]

		if group == nil {
			groupMap[portKey] = &portGroup{first: current, last: current}
			// record order that groupMap keys are created
			groupMapKeys = append(groupMapKeys, portKey)
			continue
		}

		if current == (group.last + 1) {
			group.last = current
			continue
		}

		result = append(result, formGroup(portKey, group.first, group.last))
		groupMap[portKey] = &portGroup{first: current, last: current}
	}

	for _, portKey := range groupMapKeys {
		g := groupMap[portKey]
		result = append(result, formGroup(portKey, g.first, g.last))
	}

	result = append(result, hostMappings...)

	return strings.Join(result, ", ")
}

func formGroup(key string, start uint32, last uint32) string {
	parts := strings.Split(key, "/")
	protocol := parts[0]
	var ip string
	if len(parts) > 1 {
		ip = parts[0]
		protocol = parts[1]
	}
	group := strconv.Itoa(int(start))

	// add range
	if start != last {
		group = fmt.Sprintf("%s-%d", group, last)
	}

	// add host ip
	if ip != "" {
		group = fmt.Sprintf("%s:%s->%s", ip, group, group)
	}

	// add protocol
	return fmt.Sprintf("%s/%s", group, protocol)
}

func comparePorts(i containers.Port, j containers.Port) bool {
	if i.ContainerPort != j.ContainerPort {
		return i.ContainerPort < j.ContainerPort
	}

	if i.HostIP != j.HostIP {
		return i.HostIP < j.HostIP
	}

	if i.HostPort != j.HostPort {
		return i.HostPort < j.HostPort
	}

	return i.Protocol < j.Protocol
}
