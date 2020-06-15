package convert

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"

	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	"github.com/docker/api/context/store"
)

const (
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

	for _, s := range project.Services {
		service := serviceConfigAciHelper(s)
		containerDefinition, err := service.getAciContainer(volumesCache)
		if err != nil {
			return containerinstance.ContainerGroup{}, err
		}
		if service.Ports != nil {
			var containerPorts []containerinstance.ContainerPort
			var groupPorts []containerinstance.Port
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
	groupDefinition.ContainerGroupProperties.Containers = &containers
	return groupDefinition, nil
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
	return containerinstance.Container{
		Name: to.StringPtr(s.Name),
		ContainerProperties: &containerinstance.ContainerProperties{
			Image: to.StringPtr(s.Image),
			Resources: &containerinstance.ResourceRequirements{
				Limits: &containerinstance.ResourceLimits{
					MemoryInGB: to.Float64Ptr(1),
					CPU:        to.Float64Ptr(1),
				},
				Requests: &containerinstance.ResourceRequests{
					MemoryInGB: to.Float64Ptr(1),
					CPU:        to.Float64Ptr(1),
				},
			},
			VolumeMounts: volumes,
		},
	}, nil

}

func ContainerGroupToContainer(containerID string, cg containerinstance.ContainerGroup, cc containerinstance.Container) (containers.Container, error) {
	memLimits := -1.
	if cc.Resources != nil &&
		cc.Resources.Limits != nil &&
		cc.Resources.Limits.MemoryInGB != nil {
		memLimits = *cc.Resources.Limits.MemoryInGB
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
		MemoryUsage: 0,
		MemoryLimit: uint64(memLimits),
		PidsCurrent: 0,
		PidsLimit:   0,
		Labels:      nil,
		Ports:       ToPorts(cg.IPAddress, *cc.Ports),
	}

	return c, nil
}
