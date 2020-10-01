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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"strings"

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

	dnsSidecarImage                = "busybox:1.31.1"
	azureFileDriverName            = "azure_file"
	volumeDriveroptsShareNameKey   = "share_name"
	volumeDriveroptsAccountNameKey = "storage_account_name"
	volumeReadOnly                 = "read_only"
	secretInlineMark               = "inline:"
)

// ToContainerGroup converts a compose project into a ACI container group
func ToContainerGroup(ctx context.Context, aciContext store.AciContext, p types.Project, storageHelper login.StorageLogin) (containerinstance.ContainerGroup, error) {
	project := projectAciHelper(p)
	containerGroupName := strings.ToLower(project.Name)
	volumesCache, volumesSlice, err := project.getAciFileVolumes(ctx, storageHelper)
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

	var containers []containerinstance.Container
	restartPolicy, err := project.getRestartPolicy()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	groupDefinition := containerinstance.ContainerGroup{
		Name:     &containerGroupName,
		Location: &aciContext.Location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType:                   containerinstance.Linux,
			Containers:               &containers,
			Volumes:                  volumes,
			ImageRegistryCredentials: &registryCreds,
			RestartPolicy:            restartPolicy,
		},
	}

	var groupPorts []containerinstance.Port
	var dnsLabelName *string
	for _, s := range project.Services {
		service := serviceConfigAciHelper(s)
		containerDefinition, err := service.getAciContainer(volumesCache)
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

		containers = append(containers, containerDefinition)
	}
	if len(groupPorts) > 0 {
		groupDefinition.ContainerGroupProperties.IPAddress = &containerinstance.IPAddress{
			Type:         containerinstance.Public,
			Ports:        &groupPorts,
			DNSNameLabel: dnsLabelName,
		}
	}
	if len(containers) > 1 {
		dnsSideCar := getDNSSidecar(containers)
		containers = append(containers, dnsSideCar)
	}
	groupDefinition.ContainerGroupProperties.Containers = &containers

	return groupDefinition, nil
}

func convertPortsToAci(service serviceConfigAciHelper) ([]containerinstance.ContainerPort, []containerinstance.Port, *string, error) {
	var groupPorts []containerinstance.Port
	var containerPorts []containerinstance.ContainerPort
	for _, portConfig := range service.Ports {
		if portConfig.Published != 0 && portConfig.Published != portConfig.Target {
			msg := fmt.Sprintf("Port mapping is not supported with ACI, cannot map port %d to %d for container %s",
				portConfig.Published, portConfig.Target, service.Name)
			return nil, nil, nil, errors.New(msg)
		}
		portNumber := int32(portConfig.Target)
		containerPorts = append(containerPorts, containerinstance.ContainerPort{
			Port: to.Int32Ptr(portNumber),
		})
		groupPorts = append(groupPorts, containerinstance.Port{
			Port:     to.Int32Ptr(portNumber),
			Protocol: containerinstance.TCP,
		})
	}
	var dnsLabelName *string = nil
	if service.DomainName != "" {
		dnsLabelName = &service.DomainName
	}
	return containerPorts, groupPorts, dnsLabelName, nil
}

func getDNSSidecar(containers []containerinstance.Container) containerinstance.Container {
	var commands []string
	for _, container := range containers {
		commands = append(commands, fmt.Sprintf("echo 127.0.0.1 %s >> /etc/hosts", *container.Name))
	}
	// ACI restart policy is currently at container group level, cannot let the sidecar terminate quietly once /etc/hosts has been edited
	// Pricing is done at the container group level so letting the sidecar container "sleep" should not impact the price for the whole group
	commands = append(commands, "sleep infinity")
	alpineCmd := []string{"sh", "-c", strings.Join(commands, ";")}
	dnsSideCar := containerinstance.Container{
		Name: to.StringPtr(ComposeDNSSidecarName),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:   to.StringPtr(dnsSidecarImage),
			Command: &alpineCmd,
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					MemoryInGB: to.Float64Ptr(0.1),  // "The memory requirement should be in incrememts of 0.1 GB."
					CPU:        to.Float64Ptr(0.01), //  "The CPU requirement should be in incrememts of 0.01."
				},
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

func (p projectAciHelper) getAciSecretVolumes() ([]containerinstance.Volume, error) {
	var secretVolumes []containerinstance.Volume
	for secretName, filepathToRead := range p.Secrets {
		var data []byte
		if strings.HasPrefix(filepathToRead.File, secretInlineMark) {
			data = []byte(filepathToRead.File[len(secretInlineMark):])
		} else {
			var err error
			data, err = ioutil.ReadFile(filepathToRead.File)
			if err != nil {
				return secretVolumes, err
			}
		}
		if len(data) == 0 {
			continue
		}
		dataStr := base64.StdEncoding.EncodeToString(data)
		secretVolumes = append(secretVolumes, containerinstance.Volume{
			Name: to.StringPtr(secretName),
			Secret: map[string]*string{
				secretName: &dataStr,
			},
		})
	}
	return secretVolumes, nil
}

func (p projectAciHelper) getAciFileVolumes(ctx context.Context, helper login.StorageLogin) (map[string]bool, []containerinstance.Volume, error) {
	azureFileVolumesMap := make(map[string]bool, len(p.Volumes))
	var azureFileVolumesSlice []containerinstance.Volume
	for name, v := range p.Volumes {
		if v.Driver == azureFileDriverName {
			shareName, ok := v.DriverOpts[volumeDriveroptsShareNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve fileshare name for Azurefile")
			}
			accountName, ok := v.DriverOpts[volumeDriveroptsAccountNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve account name for Azurefile")
			}
			readOnly, ok := v.DriverOpts[volumeReadOnly]
			if !ok {
				readOnly = "false"
			}
			ro, err := strconv.ParseBool(readOnly)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid mode %q for volume", readOnly)
			}
			accountKey, err := helper.GetAzureStorageAccountKey(ctx, accountName)
			if err != nil {
				return nil, nil, err
			}
			aciVolume := containerinstance.Volume{
				Name: to.StringPtr(name),
				AzureFile: &containerinstance.AzureFileVolume{
					ShareName:          to.StringPtr(shareName),
					StorageAccountName: to.StringPtr(accountName),
					StorageAccountKey:  to.StringPtr(accountKey),
					ReadOnly:           &ro,
				},
			}
			azureFileVolumesMap[name] = true
			azureFileVolumesSlice = append(azureFileVolumesSlice, aciVolume)
		}
	}
	return azureFileVolumesMap, azureFileVolumesSlice, nil
}

func (p projectAciHelper) getRestartPolicy() (containerinstance.ContainerGroupRestartPolicy, error) {
	var restartPolicyCondition containerinstance.ContainerGroupRestartPolicy
	if len(p.Services) >= 1 {
		alreadySpecified := false
		restartPolicyCondition = containerinstance.Always
		for _, service := range p.Services {
			if service.Deploy != nil &&
				service.Deploy.RestartPolicy != nil {
				if !alreadySpecified {
					alreadySpecified = true
					restartPolicyCondition = toAciRestartPolicy(service.Deploy.RestartPolicy.Condition)
				}
				if alreadySpecified && restartPolicyCondition != toAciRestartPolicy(service.Deploy.RestartPolicy.Condition) {
					return "", errors.New("ACI integration does not support specifying different restart policies on services in the same compose application")
				}

			}
		}
	}
	return restartPolicyCondition, nil
}

func toAciRestartPolicy(restartPolicy string) containerinstance.ContainerGroupRestartPolicy {
	switch restartPolicy {
	case containers.RestartPolicyNone:
		return containerinstance.Never
	case containers.RestartPolicyAny:
		return containerinstance.Always
	case containers.RestartPolicyOnFailure:
		return containerinstance.OnFailure
	default:
		return containerinstance.Always
	}
}

func toContainerRestartPolicy(aciRestartPolicy containerinstance.ContainerGroupRestartPolicy) string {
	switch aciRestartPolicy {
	case containerinstance.Never:
		return containers.RestartPolicyNone
	case containerinstance.Always:
		return containers.RestartPolicyAny
	case containerinstance.OnFailure:
		return containers.RestartPolicyOnFailure
	default:
		return containers.RestartPolicyAny
	}
}

type serviceConfigAciHelper types.ServiceConfig

func (s serviceConfigAciHelper) getAciFileVolumeMounts(volumesCache map[string]bool) ([]containerinstance.VolumeMount, error) {
	var aciServiceVolumes []containerinstance.VolumeMount
	for _, sv := range s.Volumes {
		if !volumesCache[sv.Source] {
			return []containerinstance.VolumeMount{}, fmt.Errorf("could not find volume source %q", sv.Source)
		}
		aciServiceVolumes = append(aciServiceVolumes, containerinstance.VolumeMount{
			Name:      to.StringPtr(sv.Source),
			MountPath: to.StringPtr(sv.Target),
		})
	}
	return aciServiceVolumes, nil
}

func (s serviceConfigAciHelper) getAciSecretsVolumeMounts() []containerinstance.VolumeMount {
	var secretVolumeMounts []containerinstance.VolumeMount
	for _, secret := range s.Secrets {
		secretsMountPath := "/run/secrets"
		if secret.Target == "" {
			secret.Target = secret.Source
		}
		// Specifically use "/" here and not filepath.Join() to avoid windows path being sent and used inside containers
		secretsMountPath = secretsMountPath + "/" + secret.Target
		vmName := strings.Split(secret.Source, "=")[0]
		vm := containerinstance.VolumeMount{
			Name:      to.StringPtr(vmName),
			MountPath: to.StringPtr(secretsMountPath),
			ReadOnly:  to.BoolPtr(true), // TODO Confirm if the secrets are read only
		}
		secretVolumeMounts = append(secretVolumeMounts, vm)
	}
	return secretVolumeMounts
}

func (s serviceConfigAciHelper) getAciContainer(volumesCache map[string]bool) (containerinstance.Container, error) {
	secretVolumeMounts := s.getAciSecretsVolumeMounts()
	aciServiceVolumes, err := s.getAciFileVolumeMounts(volumesCache)
	if err != nil {
		return containerinstance.Container{}, err
	}
	allVolumes := append(aciServiceVolumes, secretVolumeMounts...)
	var volumes *[]containerinstance.VolumeMount
	if len(allVolumes) == 0 {
		volumes = nil
	} else {
		volumes = &allVolumes
	}

	memLimit := 1. // Default 1 Gb
	var cpuLimit float64 = 1
	if s.Deploy != nil && s.Deploy.Resources.Limits != nil {
		if s.Deploy.Resources.Limits.MemoryBytes != 0 {
			memLimit = bytesToGb(s.Deploy.Resources.Limits.MemoryBytes)
		}
		if s.Deploy.Resources.Limits.NanoCPUs != "" {
			cpuLimit, err = strconv.ParseFloat(s.Deploy.Resources.Limits.NanoCPUs, 0)
			if err != nil {
				return containerinstance.Container{}, err
			}
		}
	}
	return containerinstance.Container{
		Name: to.StringPtr(s.Name),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image:                to.StringPtr(s.Image),
			Command:              to.StringSlicePtr(s.Command),
			EnvironmentVariables: getEnvVariables(s.Environment),
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					MemoryInGB: to.Float64Ptr(memLimit),
					CPU:        to.Float64Ptr(cpuLimit),
				},
				Requests: &containerinstance.ResourceRequests{
					MemoryInGB: to.Float64Ptr(memLimit), // TODO: use the memory requests here and not limits
					CPU:        to.Float64Ptr(cpuLimit), // TODO: use the cpu requests here and not limits
				},
			},
			VolumeMounts: volumes,
		},
	}, nil
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

func bytesToGb(b types.UnitBytes) float64 {
	f := float64(b) / 1024 / 1024 / 1024 // from bytes to gigabytes
	return math.Round(f*100) / 100
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
	memLimits := 0.
	if cc.Resources != nil &&
		cc.Resources.Limits != nil &&
		cc.Resources.Limits.MemoryInGB != nil {
		memLimits = *cc.Resources.Limits.MemoryInGB * 1024 * 1024 * 1024
	}

	cpuLimit := 0.
	if cc.Resources != nil &&
		cc.Resources.Limits != nil &&
		cc.Resources.Limits.CPU != nil {
		cpuLimit = *cc.Resources.Limits.CPU
	}

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

	var config *containers.RuntimeConfig = &containers.RuntimeConfig{FQDN: fqdn(cg, region)}
	if envVars != nil {
		config.Env = envVars
	}
	c := containers.Container{
		ID:                     containerID,
		Status:                 status,
		Image:                  to.String(cc.Image),
		Command:                command,
		CPUTime:                0,
		CPULimit:               cpuLimit,
		MemoryUsage:            0,
		MemoryLimit:            uint64(memLimits),
		PidsCurrent:            0,
		PidsLimit:              0,
		Labels:                 nil,
		Ports:                  ToPorts(cg.IPAddress, *cc.Ports),
		Platform:               platform,
		RestartPolicyCondition: toContainerRestartPolicy(cg.RestartPolicy),
		Config:                 config,
	}

	return c
}

// GetStatus returns status for the specified container
func GetStatus(container containerinstance.Container, group containerinstance.ContainerGroup) string {
	status := compose.UNKNOWN
	if group.InstanceView != nil && group.InstanceView.State != nil {
		status = "Node " + *group.InstanceView.State
	}
	if container.InstanceView != nil && container.InstanceView.CurrentState != nil {
		status = *container.InstanceView.CurrentState.State
	}
	return status
}
