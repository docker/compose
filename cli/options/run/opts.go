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

package run

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/compose-cli/utils"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/cli/opts"
	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/docker/go-connections/nat"
	specs "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/docker/compose-cli/api/containers"
)

// Opts contain run command options
type Opts struct {
	Name                   string
	Command                []string
	Publish                []string
	Labels                 []string
	Volumes                []string
	Cpus                   float64
	Memory                 utils.MemBytes
	Detach                 bool
	Environment            []string
	EnvironmentFiles       []string
	RestartPolicyCondition string
	DomainName             string
	Rm                     bool
	HealthCmd              string
	HealthInterval         time.Duration
	HealthRetries          int
	HealthStartPeriod      time.Duration
	HealthTimeout          time.Duration
	Platform               string
}

// RestartPolicyList all available restart policy values
var RestartPolicyList = []string{containers.RestartPolicyRunNo, containers.RestartPolicyRunAlways, containers.RestartPolicyOnFailure}

// ToContainerConfig convert run options to a container configuration
func (r *Opts) ToContainerConfig(image string) (containers.ContainerConfig, error) {
	if r.Name == "" {
		r.Name = getRandomName()
	}

	publish, err := r.toPorts()
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	labels, err := toLabels(r.Labels)
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	restartPolicy, err := toRestartPolicy(r.RestartPolicyCondition)
	if err != nil {
		return containers.ContainerConfig{}, err
	}

	envVars := r.Environment
	for _, f := range r.EnvironmentFiles {
		vars, err := opts.ParseEnvFile(f)
		if err != nil {
			return containers.ContainerConfig{}, err
		}
		envVars = append(envVars, vars...)
	}

	var platform *specs.Platform

	if r.Platform != "" {
		p, err := platforms.Parse(r.Platform)
		if err != nil {
			return containers.ContainerConfig{}, err
		}
		platform = &p
	}

	return containers.ContainerConfig{
		ID:                     r.Name,
		Image:                  image,
		Command:                r.Command,
		Ports:                  publish,
		Labels:                 labels,
		Volumes:                r.Volumes,
		MemLimit:               r.Memory,
		CPULimit:               r.Cpus,
		Environment:            envVars,
		RestartPolicyCondition: restartPolicy,
		DomainName:             r.DomainName,
		AutoRemove:             r.Rm,
		Healthcheck:            r.toHealthcheck(),
		Platform:               platform,
	}, nil
}

func (r *Opts) toHealthcheck() containers.Healthcheck {
	var healthCmd []string

	if len(r.HealthCmd) > 0 {
		healthCmd = strings.Split(r.HealthCmd, " ")
	}
	return containers.Healthcheck{
		Disable:     len(healthCmd) == 0,
		Test:        healthCmd,
		Interval:    types.Duration(r.HealthInterval),
		StartPeriod: types.Duration(r.HealthStartPeriod),
		Timeout:     types.Duration(r.HealthTimeout),
		Retries:     r.HealthRetries,
	}
}

var restartPolicyMap = map[string]string{
	"":                                containers.RestartPolicyNone,
	containers.RestartPolicyNone:      containers.RestartPolicyNone,
	containers.RestartPolicyAny:       containers.RestartPolicyAny,
	containers.RestartPolicyOnFailure: containers.RestartPolicyOnFailure,

	containers.RestartPolicyRunNo:     containers.RestartPolicyNone,
	containers.RestartPolicyRunAlways: containers.RestartPolicyAny,
}

func toRestartPolicy(value string) (string, error) {
	value, ok := restartPolicyMap[value]
	if !ok {
		return "", fmt.Errorf("invalid restart value, must be one of %s", strings.Join(RestartPolicyList, ", "))
	}
	return value, nil
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

func getRandomName() string {
	// Azure supports hyphen but not underscore in names
	return strings.Replace(namesgenerator.GetRandomName(0), "_", "-", -1)
}
