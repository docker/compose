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

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/versions"
)

// ToMobyEnv convert into []string
func ToMobyEnv(environment compose.MappingWithEquals) []string {
	var env []string
	for k, v := range environment {
		if v == nil {
			env = append(env, k)
		} else {
			env = append(env, fmt.Sprintf("%s=%s", k, *v))
		}
	}
	return env
}

// ToMobyHealthCheck convert into container.HealthConfig
func (s *composeService) ToMobyHealthCheck(ctx context.Context, check *compose.HealthCheckConfig) (*container.HealthConfig, error) {
	if check == nil {
		return nil, nil
	}
	var (
		interval time.Duration
		timeout  time.Duration
		period   time.Duration
		retries  int
	)
	if check.Interval != nil {
		interval = time.Duration(*check.Interval)
	}
	if check.Timeout != nil {
		timeout = time.Duration(*check.Timeout)
	}
	if check.StartPeriod != nil {
		period = time.Duration(*check.StartPeriod)
	}
	if check.Retries != nil {
		retries = int(*check.Retries)
	}
	test := check.Test
	if check.Disable {
		test = []string{"NONE"}
	}
	var startInterval time.Duration
	if check.StartInterval != nil {
		version, err := s.RuntimeVersion(ctx)
		if err != nil {
			return nil, err
		}
		if versions.LessThan(version, "1.44") {
			return nil, errors.New("can't set healthcheck.start_interval as feature require Docker Engine v25 or later")
		} else {
			startInterval = time.Duration(*check.StartInterval)
		}
	}
	return &container.HealthConfig{
		Test:          test,
		Interval:      interval,
		Timeout:       timeout,
		StartPeriod:   period,
		StartInterval: startInterval,
		Retries:       retries,
	}, nil
}

// ToSeconds convert into seconds
func ToSeconds(d *compose.Duration) *int {
	if d == nil {
		return nil
	}
	s := int(time.Duration(*d).Seconds())
	return &s
}

func (s *composeService) toComposeHealthCheck(healthConfig *container.HealthConfig) *compose.HealthCheckConfig {
	var healthCheck compose.HealthCheckConfig
	healthCheck.Test = healthConfig.Test
	if healthConfig.Timeout != 0 {
		timeout := compose.Duration(healthConfig.Timeout)
		healthCheck.Timeout = &timeout
	}
	if healthConfig.Interval != 0 {
		interval := compose.Duration(healthConfig.Interval)
		healthCheck.Interval = &interval
	}
	if healthConfig.StartPeriod != 0 {
		startPeriod := compose.Duration(healthConfig.StartPeriod)
		healthCheck.StartPeriod = &startPeriod
	}
	if healthConfig.StartInterval != 0 {
		startInterval := compose.Duration(healthConfig.StartInterval)
		healthCheck.StartInterval = &startInterval
	}
	if healthConfig.Retries != 0 {
		retries := uint64(healthConfig.Retries)
		healthCheck.Retries = &retries
	}
	return &healthCheck
}

func (s *composeService) toComposeVolumes(volumes []types.MountPoint) (map[string]compose.VolumeConfig,
	[]compose.ServiceVolumeConfig, map[string]compose.SecretConfig, []compose.ServiceSecretConfig) {
	volumeConfigs := make(map[string]compose.VolumeConfig)
	secretConfigs := make(map[string]compose.SecretConfig)
	var serviceVolumeConfigs []compose.ServiceVolumeConfig
	var serviceSecretConfigs []compose.ServiceSecretConfig

	for _, volume := range volumes {
		serviceVC := compose.ServiceVolumeConfig{
			Type:     string(volume.Type),
			Source:   volume.Source,
			Target:   volume.Destination,
			ReadOnly: !volume.RW,
		}
		switch volume.Type {
		case mount.TypeVolume:
			serviceVC.Source = volume.Name
			vol := compose.VolumeConfig{}
			if volume.Driver != "local" {
				vol.Driver = volume.Driver
				vol.Name = volume.Name
			}
			volumeConfigs[volume.Name] = vol
			serviceVolumeConfigs = append(serviceVolumeConfigs, serviceVC)
		case mount.TypeBind:
			if strings.HasPrefix(volume.Destination, "/run/secrets") {
				destination := strings.Split(volume.Destination, "/")
				secret := compose.SecretConfig{
					Name: destination[len(destination)-1],
					File: strings.TrimPrefix(volume.Source, "/host_mnt"),
				}
				secretConfigs[secret.Name] = secret
				serviceSecretConfigs = append(serviceSecretConfigs, compose.ServiceSecretConfig{
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
