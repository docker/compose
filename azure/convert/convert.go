/*
   Copyright 2020 Docker, Inc.

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
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"

	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	"github.com/docker/api/context/store"
)

const (
	// ComposeDNSSidecarName name of the dns sidecar container
	ComposeDNSSidecarName = "aci--dns--sidecar"
	dnsSidecarImage       = "busybox:1.31.1"

	azureFileDriverName            = "azure_file"
	volumeDriveroptsShareNameKey   = "share_name"
	volumeDriveroptsAccountNameKey = "storage_account_name"
	volumeDriveroptsAccountKeyKey  = "storage_account_key"
	secretInlineMark               = "inline:"
)

// ToContainerGroup converts a compose project into a ACI container group
func ToContainerGroup(aciContext store.AciContext, p compose.Project) (containerinstance.ContainerGroup, error) {
	project := projectAciHelper(p)
	containerGroupName := strings.ToLower(project.Name)
	volumesCache, volumesSlice, err := project.getAciFileVolumes()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	secretVolumes, err := project.getAciSecretVolumes()
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}
	allVolumes := append(volumesSlice, secretVolumes...)
	var volumes *[]containerinstance.Volume
	if len(allVolumes) == 0 {
		volumes = nil
	} else {
		volumes = &allVolumes
	}

	registryCreds, err := getRegistryCredentials(p, newCliRegistryConfLoader())
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}

	var containers []containerinstance.Container
	groupDefinition := containerinstance.ContainerGroup{
		Name:     &containerGroupName,
		Location: &aciContext.Location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType:                   containerinstance.Linux,
			Containers:               &containers,
			Volumes:                  volumes,
			ImageRegistryCredentials: &registryCreds,
		},
	}

	var groupPorts []containerinstance.Port
	for _, s := range project.Services {
		service := serviceConfigAciHelper(s)
		containerDefinition, err := service.getAciContainer(volumesCache)
		if err != nil {
			return containerinstance.ContainerGroup{}, err
		}
		if service.Ports != nil {
			var containerPorts []containerinstance.ContainerPort
			for _, portConfig := range service.Ports {
				if portConfig.Published != 0 && portConfig.Published != portConfig.Target {
					msg := fmt.Sprintf("Port mapping is not supported with ACI, cannot map port %d to %d for container %s",
						portConfig.Published, portConfig.Target, service.Name)
					return groupDefinition, errors.New(msg)
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
			containerDefinition.ContainerProperties.Ports = &containerPorts
			groupDefinition.ContainerGroupProperties.IPAddress = &containerinstance.IPAddress{
				Type:  containerinstance.Public,
				Ports: &groupPorts,
			}
		}

		containers = append(containers, containerDefinition)
	}
	if len(containers) > 1 {
		dnsSideCar := getDNSSidecar(containers)
		containers = append(containers, dnsSideCar)
	}
	groupDefinition.ContainerGroupProperties.Containers = &containers

	return groupDefinition, nil
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

type projectAciHelper compose.Project

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

func (p projectAciHelper) getAciFileVolumes() (map[string]bool, []containerinstance.Volume, error) {
	azureFileVolumesMap := make(map[string]bool, len(p.Volumes))
	var azureFileVolumesSlice []containerinstance.Volume
	for name, v := range p.Volumes {
		if v.Driver == azureFileDriverName {
			shareName, ok := v.DriverOpts[volumeDriveroptsShareNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve share name for Azurefile")
			}
			accountName, ok := v.DriverOpts[volumeDriveroptsAccountNameKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve account name for Azurefile")
			}
			accountKey, ok := v.DriverOpts[volumeDriveroptsAccountKeyKey]
			if !ok {
				return nil, nil, fmt.Errorf("cannot retrieve account key for Azurefile")
			}
			aciVolume := containerinstance.Volume{
				Name: to.StringPtr(name),
				AzureFile: &containerinstance.AzureFileVolume{
					ShareName:          to.StringPtr(shareName),
					StorageAccountName: to.StringPtr(accountName),
					StorageAccountKey:  to.StringPtr(accountKey),
				},
			}
			azureFileVolumesMap[name] = true
			azureFileVolumesSlice = append(azureFileVolumesSlice, aciVolume)
		}
	}
	return azureFileVolumesMap, azureFileVolumesSlice, nil
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
			Image: to.StringPtr(s.Image),
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

func bytesToGb(b types.UnitBytes) float64 {
	f := float64(b) / 1024 / 1024 / 1024 // from bytes to gigabytes
	return math.Round(f*100) / 100
}

// ContainerGroupToContainer composes a Container from an ACI container definition
func ContainerGroupToContainer(containerID string, cg containerinstance.ContainerGroup, cc containerinstance.Container) (containers.Container, error) {
	memLimits := 0.
	if cc.Resources != nil &&
		cc.Resources.Limits != nil &&
		cc.Resources.Limits.MemoryInGB != nil {
		memLimits = *cc.Resources.Limits.MemoryInGB
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

	status := "Unknown"
	if cc.InstanceView != nil && cc.InstanceView.CurrentState != nil {
		status = *cc.InstanceView.CurrentState.State
	}

	c := containers.Container{
		ID:          containerID,
		Status:      status,
		Image:       to.String(cc.Image),
		Command:     command,
		CPUTime:     0,
		CPULimit:    cpuLimit,
		MemoryUsage: 0,
		MemoryLimit: uint64(memLimits),
		PidsCurrent: 0,
		PidsLimit:   0,
		Labels:      nil,
		Ports:       ToPorts(cg.IPAddress, *cc.Ports),
	}

	return c, nil
}
