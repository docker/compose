/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package moby

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"

	"github.com/docker/compose-cli/api/containers"
)

// ToRuntimeConfig convert into containers.RuntimeConfig
func ToRuntimeConfig(m *types.ContainerJSON) *containers.RuntimeConfig {
	if m.Config == nil {
		return nil
	}
	var env map[string]string
	if m.Config.Env != nil {
		env = make(map[string]string)
		for _, e := range m.Config.Env {
			tokens := strings.Split(e, "=")
			if len(tokens) != 2 {
				continue
			}
			env[tokens[0]] = tokens[1]
		}
	}

	var labels []string
	if m.Config.Labels != nil {
		for k, v := range m.Config.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
	}
	sort.Strings(labels)

	if env == nil &&
		labels == nil {
		return nil
	}

	return &containers.RuntimeConfig{
		Env:    env,
		Labels: labels,
	}
}

// ToHostConfig convert into containers.HostConfig
func ToHostConfig(m *types.ContainerJSON) *containers.HostConfig {
	if m.HostConfig == nil {
		return nil
	}

	return &containers.HostConfig{
		AutoRemove:    m.HostConfig.AutoRemove,
		RestartPolicy: fromRestartPolicyName(m.HostConfig.RestartPolicy.Name),
		CPULimit:      float64(m.HostConfig.Resources.NanoCPUs) / 1e9,
		MemoryLimit:   uint64(m.HostConfig.Resources.Memory),
	}
}

// ToPorts convert into containers.Port
func ToPorts(ports []types.Port) []containers.Port {
	result := []containers.Port{}
	for _, port := range ports {
		result = append(result, containers.Port{
			ContainerPort: uint32(port.PrivatePort),
			HostPort:      uint32(port.PublicPort),
			HostIP:        port.IP,
			Protocol:      port.Type,
		})
	}

	return result
}

// FromPorts convert to nat.Port / nat.PortBinding
func FromPorts(ports []containers.Port) (map[nat.Port]struct{}, map[nat.Port][]nat.PortBinding, error) {
	var (
		exposedPorts = make(map[nat.Port]struct{}, len(ports))
		bindings     = make(map[nat.Port][]nat.PortBinding)
	)

	for _, port := range ports {
		p, err := nat.NewPort(port.Protocol, strconv.Itoa(int(port.ContainerPort)))
		if err != nil {
			return nil, nil, err
		}

		if _, exists := exposedPorts[p]; !exists {
			exposedPorts[p] = struct{}{}
		}

		portBinding := nat.PortBinding{
			HostIP:   port.HostIP,
			HostPort: strconv.Itoa(int(port.HostPort)),
		}
		bslice, exists := bindings[p]
		if !exists {
			bslice = []nat.PortBinding{}
		}
		bindings[p] = append(bslice, portBinding)
	}

	return exposedPorts, bindings, nil
}

func fromRestartPolicyName(m string) string {
	switch m {
	case "always":
		return containers.RestartPolicyAny
	case "on-failure":
		return containers.RestartPolicyOnFailure
	case "no", "":
		fallthrough
	default:
		return containers.RestartPolicyNone
	}
}

// ToRestartPolicy convert to container.RestartPolicy
func ToRestartPolicy(p string) container.RestartPolicy {
	switch p {
	case containers.RestartPolicyAny:
		return container.RestartPolicy{Name: "always"}
	case containers.RestartPolicyOnFailure:
		return container.RestartPolicy{Name: "on-failure"}
	case containers.RestartPolicyNone:
		fallthrough
	default:
		return container.RestartPolicy{Name: "no"}
	}
}
