/*
   Copyright 2020 The Compose Specification Authors.

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

package types

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/distribution/distribution/v3/reference"
	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"
)

// Project is the result of loading a set of compose files
type Project struct {
	Name         string            `yaml:"name,omitempty" json:"name,omitempty"`
	WorkingDir   string            `yaml:"-" json:"-"`
	Services     Services          `json:"services"`
	Networks     Networks          `yaml:",omitempty" json:"networks,omitempty"`
	Volumes      Volumes           `yaml:",omitempty" json:"volumes,omitempty"`
	Secrets      Secrets           `yaml:",omitempty" json:"secrets,omitempty"`
	Configs      Configs           `yaml:",omitempty" json:"configs,omitempty"`
	Extensions   Extensions        `yaml:",inline" json:"-"` // https://github.com/golang/go/issues/6213
	ComposeFiles []string          `yaml:"-" json:"-"`
	Environment  map[string]string `yaml:"-" json:"-"`

	// DisabledServices track services which have been disable as profile is not active
	DisabledServices Services `yaml:"-" json:"-"`
}

// ServiceNames return names for all services in this Compose config
func (p Project) ServiceNames() []string {
	var names []string
	for _, s := range p.Services {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// VolumeNames return names for all volumes in this Compose config
func (p Project) VolumeNames() []string {
	var names []string
	for k := range p.Volumes {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// NetworkNames return names for all volumes in this Compose config
func (p Project) NetworkNames() []string {
	var names []string
	for k := range p.Networks {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// SecretNames return names for all secrets in this Compose config
func (p Project) SecretNames() []string {
	var names []string
	for k := range p.Secrets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ConfigNames return names for all configs in this Compose config
func (p Project) ConfigNames() []string {
	var names []string
	for k := range p.Configs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// GetServices retrieve services by names, or return all services if no name specified
func (p Project) GetServices(names ...string) (Services, error) {
	if len(names) == 0 {
		return p.Services, nil
	}
	services := Services{}
	for _, name := range names {
		var serviceConfig *ServiceConfig
		for _, s := range p.Services {
			if s.Name == name {
				serviceConfig = &s
				break
			}
		}
		if serviceConfig == nil {
			return services, fmt.Errorf("no such service: %s", name)
		}
		services = append(services, *serviceConfig)
	}
	return services, nil
}

// GetService retrieve a specific service by name
func (p Project) GetService(name string) (ServiceConfig, error) {
	services, err := p.GetServices(name)
	if err != nil {
		return ServiceConfig{}, err
	}
	if len(services) == 0 {
		return ServiceConfig{}, fmt.Errorf("no such service: %s", name)
	}
	return services[0], nil
}

func (p Project) AllServices() Services {
	var all Services
	all = append(all, p.Services...)
	all = append(all, p.DisabledServices...)
	return all
}

type ServiceFunc func(service ServiceConfig) error

// WithServices run ServiceFunc on each service and dependencies in dependency order
func (p Project) WithServices(names []string, fn ServiceFunc) error {
	return p.withServices(names, fn, map[string]bool{})
}

func (p Project) withServices(names []string, fn ServiceFunc, done map[string]bool) error {
	services, err := p.GetServices(names...)
	if err != nil {
		return err
	}
	for _, service := range services {
		if done[service.Name] {
			continue
		}
		dependencies := service.GetDependencies()
		if len(dependencies) > 0 {
			err := p.withServices(dependencies, fn, done)
			if err != nil {
				return err
			}
		}
		if err := fn(service); err != nil {
			return err
		}
		done[service.Name] = true
	}
	return nil
}

// RelativePath resolve a relative path based project's working directory
func (p *Project) RelativePath(path string) string {
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(p.WorkingDir, path)
}

// HasProfile return true if service has no profile declared or has at least one profile matching
func (s ServiceConfig) HasProfile(profiles []string) bool {
	if len(s.Profiles) == 0 {
		return true
	}
	for _, p := range profiles {
		for _, sp := range s.Profiles {
			if sp == p {
				return true
			}
		}
	}
	return false
}

// GetProfiles retrieve the profiles implicitly enabled by explicitly targeting selected services
func (s Services) GetProfiles() []string {
	set := map[string]struct{}{}
	for _, service := range s {
		for _, p := range service.Profiles {
			set[p] = struct{}{}
		}
	}
	var profiles []string
	for k := range set {
		profiles = append(profiles, k)
	}
	return profiles
}

// ApplyProfiles disables service which don't match selected profiles
func (p *Project) ApplyProfiles(profiles []string) {
	for _, p := range profiles {
		if p == "*" {
			return
		}
	}
	var enabled, disabled Services
	for _, service := range p.Services {
		if service.HasProfile(profiles) {
			enabled = append(enabled, service)
		} else {
			disabled = append(disabled, service)
		}
	}
	p.Services = enabled
	p.DisabledServices = disabled
}

// WithoutUnnecessaryResources drops networks/volumes/secrets/configs that are not referenced by active services
func (p *Project) WithoutUnnecessaryResources() {
	requiredNetworks := map[string]struct{}{}
	requiredVolumes := map[string]struct{}{}
	requiredSecrets := map[string]struct{}{}
	requiredConfigs := map[string]struct{}{}
	for _, s := range p.Services {
		for k := range s.Networks {
			requiredNetworks[k] = struct{}{}
		}
		for _, v := range s.Volumes {
			if v.Type != VolumeTypeVolume || v.Source == "" {
				continue
			}
			requiredVolumes[v.Source] = struct{}{}
		}
		for _, v := range s.Secrets {
			requiredSecrets[v.Source] = struct{}{}
		}
		for _, v := range s.Configs {
			requiredConfigs[v.Source] = struct{}{}
		}
	}

	networks := Networks{}
	for k := range requiredNetworks {
		networks[k] = p.Networks[k]
	}
	p.Networks = networks

	volumes := Volumes{}
	for k := range requiredVolumes {
		volumes[k] = p.Volumes[k]
	}
	p.Volumes = volumes

	secrets := Secrets{}
	for k := range requiredSecrets {
		secrets[k] = p.Secrets[k]
	}
	p.Secrets = secrets

	configs := Configs{}
	for k := range requiredConfigs {
		configs[k] = p.Configs[k]
	}
	p.Configs = configs
}

// ForServices restrict the project model to a subset of services
func (p *Project) ForServices(names []string) error {
	if len(names) == 0 {
		// All services
		return nil
	}

	set := map[string]struct{}{}
	err := p.WithServices(names, func(service ServiceConfig) error {
		set[service.Name] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}

	// Disable all services which are not explicit target or dependencies
	var enabled Services
	for _, s := range p.Services {
		if _, ok := set[s.Name]; ok {
			enabled = append(enabled, s)
		} else {
			p.DisabledServices = append(p.DisabledServices, s)
		}
	}
	p.Services = enabled
	return nil
}

// ResolveImages updates services images to include digest computed by a resolver function
func (p *Project) ResolveImages(resolver func(named reference.Named) (digest.Digest, error)) error {
	eg := errgroup.Group{}
	for i, s := range p.Services {
		idx := i
		service := s

		if service.Image == "" {
			continue
		}
		eg.Go(func() error {
			named, err := reference.ParseDockerRef(service.Image)
			if err != nil {
				return err
			}

			if _, ok := named.(reference.Canonical); !ok {
				// image is named but not digested reference
				digest, err := resolver(named)
				if err != nil {
					return err
				}
				named, err = reference.WithDigest(named, digest)
				if err != nil {
					return err
				}
			}

			service.Image = named.String()
			p.Services[idx] = service
			return nil
		})
	}
	return eg.Wait()
}
