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

package convert

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/aci/login"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/utils/formatter"
)

const (
	// StatusRunning name of the ACI running status
	StatusRunning = "Running"
	// ComposeDNSSidecarName name of the dns sidecar container
	ComposeDNSSidecarName = "aci--dns--sidecar"

	dnsSidecarImage = "docker/aci-hostnames-sidecar:1.0"
)

// ToContainerGroup converts a compose project into a ACI container group
func ToContainerGroup(ctx context.Context, aciContext store.AciContext, p types.Project, storageHelper login.StorageLogin) (containerinstance.ContainerGroup, error) {
	project := projectAciHelper(p)
	containerGroupName := strings.ToLower(project.Name)
	volumesSlice, err := project.getAciFileVolumes(ctx, storageHelper)
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	secretVolumes, err := project.getAciSecretVolumes()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	allVolumes := append(volumesSlice, secretVolumes...)
	var volumes *[]containerinstance.Volume
	if len(allVolumes) > 0 {
		volumes = &allVolumes
	}

	registryCreds, err := getRegistryCredentials(p, newCliRegistryConfLoader())
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}

	var ctnrs []containerinstance.Container
	restartPolicy, err := project.getRestartPolicy()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	groupDefinition := containerinstance.ContainerGroup{
		Name:     &containerGroupName,
		Location: &aciContext.Location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType:                   containerinstance.Linux,
			Containers:               &ctnrs,
			Volumes:                  volumes,
			ImageRegistryCredentials: &registryCreds,
			RestartPolicy:            restartPolicy,
		},
	}

	var groupPorts []containerinstance.Port
	var dnsLabelName *string
	for _, s := range project.Services {
		service := serviceConfigAciHelper(s)
		containerDefinition, err := service.getAciContainer()
		if err != nil {
			return containerinstance.ContainerGroup{}, err
		}
		if service.Labels != nil && len(service.Labels) > 0 {
			return containerinstance.ContainerGroup{}, errors.New("ACI integration does not support labels in compose applications")
		}

		containerPorts, serviceGroupPorts, serviceDomainName, err := convertPortsToAci(service)
		if err != nil {
			return groupDefinition, err
		}
		containerDefinition.ContainerProperties.Ports = &containerPorts
		groupPorts = append(groupPorts, serviceGroupPorts...)
		if serviceDomainName != nil {
			if dnsLabelName != nil && *serviceDomainName != *dnsLabelName {
				return containerinstance.ContainerGroup{}, fmt.Errorf("ACI integration does not support specifying different domain names on services in the same compose application")
			}
			dnsLabelName = serviceDomainName
		}

		ctnrs = append(ctnrs, containerDefinition)
	}
	if len(groupPorts) > 0 {
		groupDefinition.ContainerGroupProperties.IPAddress = &containerinstance.IPAddress{
			Type:         containerinstance.Public,
			Ports:        &groupPorts,
			DNSNameLabel: dnsLabelName,
		}
	}
	if len(ctnrs) > 1 {
		dnsSideCar := getDNSSidecar(ctnrs)
		ctnrs = append(ctnrs, dnsSideCar)
	}
	groupDefinition.ContainerGroupProperties.Containers = &ctnrs

	return groupDefinition, nil
}

func durationToSeconds(d *types.Duration) *int32 {
	if d == nil || *d == 0 {
		return nil
	}
	v := int32(time.Duration(*d).Seconds())
	return &v
}

func getDNSSidecar(containers []containerinstance.Container) containerinstance.Container {
	names := []string{"/hosts"}
	for _, container := range containers {
		names = append(names, *container.Name)
	}
	dnsSideCar := containerinstance.Container{
		Name: to.StringPtr(ComposeDNSSidecarName),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:   to.StringPtr(dnsSidecarImage),
			Command: &names,
			Resources: &containerinstance.ResourceRequirements{
				Requests: &containerinstance.ResourceRequests{
					MemoryInGB: to.Float64Ptr(0.1),
					CPU:        to.Float64Ptr(0.01),
				},
			},
		},
	}
	return dnsSideCar
}

type projectAciHelper types.Project

type serviceConfigAciHelper types.ServiceConfig

func (s serviceConfigAciHelper) getAciContainer() (containerinstance.Container, error) {
	aciServiceVolumes, err := s.getAciFileVolumeMounts()
	if err != nil {
		return containerinstance.Container{}, err
	}
	serviceSecretVolumes, err := s.getAciSecretsVolumeMounts()
	if err != nil {
		return containerinstance.Container{}, err
	}
	allVolumes := append(aciServiceVolumes, serviceSecretVolumes...)
	var volumes *[]containerinstance.VolumeMount
	if len(allVolumes) > 0 {
		volumes = &allVolumes
	}

	resource, err := s.getResourceRequestsLimits()
	if err != nil {
		return containerinstance.Container{}, err
	}

	return containerinstance.Container{
		Name: to.StringPtr(s.Name),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:                to.StringPtr(s.Image),
			Command:              to.StringSlicePtr(s.Command),
			EnvironmentVariables: getEnvVariables(s.Environment),
			Resources:            resource,
			VolumeMounts:         volumes,
			LivenessProbe:        s.getLivenessProbe(),
		},
	}, nil
}

func (s serviceConfigAciHelper) getResourceRequestsLimits() (*containerinstance.ResourceRequirements, error) {
	memRequest := 1. // Default 1 Gb
	var cpuRequest float64 = 1
	var err error
	hasMemoryRequest := func() bool {
		return s.Deploy != nil && s.Deploy.Resources.Reservations != nil && s.Deploy.Resources.Reservations.MemoryBytes != 0
	}
	hasCPURequest := func() bool {
		return s.Deploy != nil && s.Deploy.Resources.Reservations != nil && s.Deploy.Resources.Reservations.NanoCPUs != ""
	}
	if hasMemoryRequest() {
		memRequest = BytesToGB(float64(s.Deploy.Resources.Reservations.MemoryBytes))
	}

	if hasCPURequest() {
		cpuRequest, err = strconv.ParseFloat(s.Deploy.Resources.Reservations.NanoCPUs, 0)
		if err != nil {
			return nil, err
		}
	}
	memLimit := memRequest
	cpuLimit := cpuRequest
	if s.Deploy != nil && s.Deploy.Resources.Limits != nil {
		if s.Deploy.Resources.Limits.MemoryBytes != 0 {
			memLimit = BytesToGB(float64(s.Deploy.Resources.Limits.MemoryBytes))
			if !hasMemoryRequest() {
				memRequest = memLimit
			}
		}
		if s.Deploy.Resources.Limits.NanoCPUs != "" {
			cpuLimit, err = strconv.ParseFloat(s.Deploy.Resources.Limits.NanoCPUs, 0)
			if err != nil {
				return nil, err
			}
			if !hasCPURequest() {
				cpuRequest = cpuLimit
			}
		}
	}
	resources := containerinstance.ResourceRequirements{
		Requests: &containerinstance.ResourceRequests{
			MemoryInGB: to.Float64Ptr(memRequest),
			CPU:        to.Float64Ptr(cpuRequest),
		},
		Limits: &containerinstance.ResourceLimits{
			MemoryInGB: to.Float64Ptr(memLimit),
			CPU:        to.Float64Ptr(cpuLimit),
		},
	}
	return &resources, nil
}

func (s serviceConfigAciHelper) getLivenessProbe() *containerinstance.ContainerProbe {
	if s.HealthCheck != nil && !s.HealthCheck.Disable && len(s.HealthCheck.Test) > 0 {
		testArray := s.HealthCheck.Test
		switch s.HealthCheck.Test[0] {
		case "NONE", "CMD", "CMD-SHELL":
			testArray = s.HealthCheck.Test[1:]
		}
		if len(testArray) == 0 {
			return nil
		}

		var retries *int32
		if s.HealthCheck.Retries != nil {
			retries = to.Int32Ptr(int32(*s.HealthCheck.Retries))
		}
		probe := containerinstance.ContainerProbe{
			Exec: &containerinstance.ContainerExec{
				Command: to.StringSlicePtr(testArray),
			},
			InitialDelaySeconds: durationToSeconds(s.HealthCheck.StartPeriod),
			PeriodSeconds:       durationToSeconds(s.HealthCheck.Interval),
			TimeoutSeconds:      durationToSeconds(s.HealthCheck.Timeout),
		}
		if retries != nil && *retries > 0 {
			probe.FailureThreshold = retries
			probe.SuccessThreshold = retries
		}
		return &probe
	}
	return nil
}

func getEnvVariables(composeEnv types.MappingWithEquals) *[]containerinstance.EnvironmentVariable {
	result := []containerinstance.EnvironmentVariable{}
	for key, value := range composeEnv {
		var strValue string
		if value == nil {
			strValue = os.Getenv(key)
		} else {
			strValue = *value
		}
		result = append(result, containerinstance.EnvironmentVariable{
			Name:  to.StringPtr(key),
			Value: to.StringPtr(strValue),
		})
	}
	return &result
}

// BytesToGB convert bytes To GB
func BytesToGB(b float64) float64 {
	f := b / 1024 / 1024 / 1024 // from bytes to gigabytes
	return math.Round(f*100) / 100
}

func gbToBytes(memInBytes float64) uint64 {
	return uint64(memInBytes * 1024 * 1024 * 1024)
}

// ContainerGroupToServiceStatus convert from an ACI container definition to service status
func ContainerGroupToServiceStatus(containerID string, group containerinstance.ContainerGroup, container containerinstance.Container, region string) compose.ServiceStatus {
	var replicas = 1
	if GetStatus(container, group) != StatusRunning {
		replicas = 0
	}
	return compose.ServiceStatus{
		ID:       containerID,
		Name:     *container.Name,
		Ports:    formatter.PortsToStrings(ToPorts(group.IPAddress, *container.Ports), fqdn(group, region)),
		Replicas: replicas,
		Desired:  1,
	}
}

func fqdn(group containerinstance.ContainerGroup, region string) string {
	fqdn := ""
	if group.IPAddress != nil && group.IPAddress.DNSNameLabel != nil && *group.IPAddress.DNSNameLabel != "" {
		fqdn = *group.IPAddress.DNSNameLabel + "." + region + ".azurecontainer.io"
	}
	return fqdn
}

// ContainerGroupToContainer composes a Container from an ACI container definition
func ContainerGroupToContainer(containerID string, cg containerinstance.ContainerGroup, cc containerinstance.Container, region string) containers.Container {
	command := ""
	if cc.Command != nil {
		command = strings.Join(*cc.Command, " ")
	}

	status := GetStatus(cc, cg)
	platform := string(cg.OsType)

	var envVars map[string]string = nil
	if cc.EnvironmentVariables != nil && len(*cc.EnvironmentVariables) != 0 {
		envVars = map[string]string{}
		for _, envVar := range *cc.EnvironmentVariables {
			envVars[*envVar.Name] = *envVar.Value
		}
	}

	hostConfig := ToHostConfig(cc, cg)
	config := &containers.RuntimeConfig{
		FQDN: fqdn(cg, region),
		Env:  envVars,
	}

	var healthcheck = containers.Healthcheck{
		Disable: true,
	}
	if cc.LivenessProbe != nil &&
		cc.LivenessProbe.Exec != nil &&
		cc.LivenessProbe.Exec.Command != nil {
		if len(*cc.LivenessProbe.Exec.Command) > 0 {
			healthcheck.Disable = false
			healthcheck.Test = *cc.LivenessProbe.Exec.Command
			if cc.LivenessProbe.PeriodSeconds != nil {
				healthcheck.Interval = types.Duration(int64(*cc.LivenessProbe.PeriodSeconds) * int64(time.Second))
			}
			if cc.LivenessProbe.SuccessThreshold != nil {
				healthcheck.Retries = int(*cc.LivenessProbe.SuccessThreshold)
			}
			if cc.LivenessProbe.TimeoutSeconds != nil {
				healthcheck.Timeout = types.Duration(int64(*cc.LivenessProbe.TimeoutSeconds) * int64(time.Second))
			}
			if cc.LivenessProbe.InitialDelaySeconds != nil {
				healthcheck.StartPeriod = types.Duration(int64(*cc.LivenessProbe.InitialDelaySeconds) * int64(time.Second))
			}
		}
	}

	c := containers.Container{
		ID:          containerID,
		Status:      status,
		Image:       to.String(cc.Image),
		Command:     command,
		CPUTime:     0,
		MemoryUsage: 0,
		PidsCurrent: 0,
		PidsLimit:   0,
		Ports:       ToPorts(cg.IPAddress, *cc.Ports),
		Platform:    platform,
		Config:      config,
		HostConfig:  hostConfig,
		Healthcheck: healthcheck,
	}

	return c
}

// ToHostConfig convert an ACI container to host config value
func ToHostConfig(cc containerinstance.Container, cg containerinstance.ContainerGroup) *containers.HostConfig {
	memLimits := uint64(0)
	memRequest := uint64(0)
	cpuLimit := 0.
	cpuReservation := 0.
	if cc.Resources != nil {
		if cc.Resources.Limits != nil {
			if cc.Resources.Limits.MemoryInGB != nil {
				memLimits = gbToBytes(*cc.Resources.Limits.MemoryInGB)
			}
			if cc.Resources.Limits.CPU != nil {
				cpuLimit = *cc.Resources.Limits.CPU
			}
		}
		if cc.Resources.Requests != nil {
			if cc.Resources.Requests.MemoryInGB != nil {
				memRequest = gbToBytes(*cc.Resources.Requests.MemoryInGB)
			}
			if cc.Resources.Requests.CPU != nil {
				cpuReservation = *cc.Resources.Requests.CPU
			}
		}
	}
	hostConfig := &containers.HostConfig{
		CPULimit:          cpuLimit,
		CPUReservation:    cpuReservation,
		MemoryLimit:       memLimits,
		MemoryReservation: memRequest,
		RestartPolicy:     toContainerRestartPolicy(cg.RestartPolicy),
	}
	return hostConfig
}

// GetStatus returns status for the specified container
func GetStatus(container containerinstance.Container, group containerinstance.ContainerGroup) string {
	status := GetGroupStatus(group)
	if container.InstanceView != nil && container.InstanceView.CurrentState != nil {
		status = *container.InstanceView.CurrentState.State
	}
	return status
}

// GetGroupStatus returns status for the container group
func GetGroupStatus(group containerinstance.ContainerGroup) string {
	if group.InstanceView != nil && group.InstanceView.State != nil {
		return "Node " + *group.InstanceView.State
	}
	return compose.UNKNOWN
}
