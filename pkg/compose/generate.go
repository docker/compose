/*
   Copyright 2023 Docker Compose CLI authors

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

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"

	"golang.org/x/exp/maps"
)

func (s *composeService) Generate(ctx context.Context, options api.GenerateOptions) (*types.Project, error) {
	filtersListNames := filters.NewArgs()
	filtersListIDs := filters.NewArgs()
	for _, containerName := range options.Containers {
		filtersListNames.Add("name", containerName)
		filtersListIDs.Add("id", containerName)
	}
	containers, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filtersListNames,
		All:     true,
	})
	if err != nil {
		return nil, err
	}

	containersByIds, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filtersListIDs,
		All:     true,
	})
	if err != nil {
		return nil, err
	}
	for _, container := range containersByIds {
		if !utils.Contains(containers, container) {
			containers = append(containers, container)
		}
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no container(s) found with the following name(s): %s", strings.Join(options.Containers, ","))
	}

	return s.createProjectFromContainers(containers, options.ProjectName)
}

func (s *composeService) createProjectFromContainers(containers []moby.Container, projectName string) (*types.Project, error) {
	project := &types.Project{}
	services := types.Services{}
	networks := types.Networks{}
	volumes := types.Volumes{}
	secrets := types.Secrets{}

	if projectName != "" {
		project.Name = projectName
	}

	for _, c := range containers {
		// if the container is from a previous Compose application, use the existing service name
		serviceLabel, ok := c.Labels[api.ServiceLabel]
		if !ok {
			serviceLabel = getCanonicalContainerName(c)
		}
		service, ok := services[serviceLabel]
		if !ok {
			service = types.ServiceConfig{
				Name:   serviceLabel,
				Image:  c.Image,
				Labels: c.Labels,
			}

		}
		service.Scale = increment(service.Scale)

		inspect, err := s.apiClient().ContainerInspect(context.Background(), c.ID)
		if err != nil {
			services[serviceLabel] = service
			continue
		}
		s.extractComposeConfiguration(&service, inspect, volumes, secrets, networks)
		service.Labels = cleanDockerPreviousLabels(service.Labels)
		services[serviceLabel] = service
	}

	project.Services = services
	project.Networks = networks
	project.Volumes = volumes
	project.Secrets = secrets
	return project, nil
}

func (s *composeService) extractComposeConfiguration(service *types.ServiceConfig, inspect moby.ContainerJSON, volumes types.Volumes, secrets types.Secrets, networks types.Networks) {
	service.Environment = types.NewMappingWithEquals(inspect.Config.Env)
	if inspect.Config.Healthcheck != nil {
		healthConfig := inspect.Config.Healthcheck
		service.HealthCheck = s.toComposeHealthCheck(healthConfig)
	}
	if len(inspect.Mounts) > 0 {
		detectedVolumes, volumeConfigs, detectedSecrets, secretsConfigs := s.toComposeVolumes(inspect.Mounts)
		service.Volumes = append(service.Volumes, volumeConfigs...)
		service.Secrets = append(service.Secrets, secretsConfigs...)
		maps.Copy(volumes, detectedVolumes)
		maps.Copy(secrets, detectedSecrets)
	}
	if len(inspect.NetworkSettings.Networks) > 0 {
		detectedNetworks, networkConfigs := s.toComposeNetwork(inspect.NetworkSettings.Networks)
		service.Networks = networkConfigs
		maps.Copy(networks, detectedNetworks)
	}
	if len(inspect.HostConfig.PortBindings) > 0 {
		for key, portBindings := range inspect.HostConfig.PortBindings {
			for _, portBinding := range portBindings {
				service.Ports = append(service.Ports, types.ServicePortConfig{
					Target:    uint32(key.Int()),
					Published: portBinding.HostPort,
					Protocol:  key.Proto(),
					HostIP:    portBinding.HostIP,
				})
			}
		}
	}
}

func (s *composeService) toComposeHealthCheck(healthConfig *containerType.HealthConfig) *types.HealthCheckConfig {
	var healthCheck types.HealthCheckConfig
	healthCheck.Test = healthConfig.Test
	if healthConfig.Timeout != 0 {
		timeout := types.Duration(healthConfig.Timeout)
		healthCheck.Timeout = &timeout
	}
	if healthConfig.Interval != 0 {
		interval := types.Duration(healthConfig.Interval)
		healthCheck.Interval = &interval
	}
	if healthConfig.StartPeriod != 0 {
		startPeriod := types.Duration(healthConfig.StartPeriod)
		healthCheck.StartPeriod = &startPeriod
	}
	if healthConfig.StartInterval != 0 {
		startInterval := types.Duration(healthConfig.StartInterval)
		healthCheck.StartInterval = &startInterval
	}
	if healthConfig.Retries != 0 {
		retries := uint64(healthConfig.Retries)
		healthCheck.Retries = &retries
	}
	return &healthCheck
}

func (s *composeService) toComposeVolumes(volumes []moby.MountPoint) (map[string]types.VolumeConfig,
	[]types.ServiceVolumeConfig, map[string]types.SecretConfig, []types.ServiceSecretConfig) {
	volumeConfigs := make(map[string]types.VolumeConfig)
	secretConfigs := make(map[string]types.SecretConfig)
	var serviceVolumeConfigs []types.ServiceVolumeConfig
	var serviceSecretConfigs []types.ServiceSecretConfig

	for _, volume := range volumes {
		serviceVC := types.ServiceVolumeConfig{
			Type:     string(volume.Type),
			Source:   volume.Source,
			Target:   volume.Destination,
			ReadOnly: !volume.RW,
		}
		switch volume.Type {
		case mount.TypeVolume:
			serviceVC.Source = volume.Name
			vol := types.VolumeConfig{}
			if volume.Driver != "local" {
				vol.Driver = volume.Driver
				vol.Name = volume.Name
			}
			volumeConfigs[volume.Name] = vol
			serviceVolumeConfigs = append(serviceVolumeConfigs, serviceVC)
		case mount.TypeBind:
			if strings.HasPrefix(volume.Destination, "/run/secrets") {
				destination := strings.Split(volume.Destination, "/")
				secret := types.SecretConfig{
					Name: destination[len(destination)-1],
					File: strings.TrimPrefix(volume.Source, "/host_mnt"),
				}
				secretConfigs[secret.Name] = secret
				serviceSecretConfigs = append(serviceSecretConfigs, types.ServiceSecretConfig{
					Source: secret.Name,
					Target: volume.Destination,
				})
			} else {
				serviceVolumeConfigs = append(serviceVolumeConfigs, serviceVC)
			}
		}
	}
	return volumeConfigs, serviceVolumeConfigs, secretConfigs, serviceSecretConfigs
}

func (s *composeService) toComposeNetwork(networks map[string]*network.EndpointSettings) (map[string]types.NetworkConfig, map[string]*types.ServiceNetworkConfig) {
	networkConfigs := make(map[string]types.NetworkConfig)
	serviceNetworkConfigs := make(map[string]*types.ServiceNetworkConfig)

	for name, net := range networks {
		inspect, err := s.apiClient().NetworkInspect(context.Background(), name, network.InspectOptions{})
		if err != nil {
			networkConfigs[name] = types.NetworkConfig{}
		} else {
			networkConfigs[name] = types.NetworkConfig{
				Internal: inspect.Internal,
			}

		}
		serviceNetworkConfigs[name] = &types.ServiceNetworkConfig{
			Aliases: net.Aliases,
		}
	}
	return networkConfigs, serviceNetworkConfigs
}

func cleanDockerPreviousLabels(labels types.Labels) types.Labels {
	cleanedLabels := types.Labels{}
	for key, value := range labels {
		if !strings.HasPrefix(key, "com.docker.compose.") && !strings.HasPrefix(key, "desktop.docker.io") {
			cleanedLabels[key] = value
		}
	}
	return cleanedLabels
}
