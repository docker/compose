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

package loader

import (
	"reflect"
	"sort"

	"github.com/compose-spec/compose-go/types"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
)

type specials struct {
	m map[reflect.Type]func(dst, src reflect.Value) error
}

var serviceSpecials = &specials{
	m: map[reflect.Type]func(dst, src reflect.Value) error{
		reflect.TypeOf(&types.LoggingConfig{}):           safelyMerge(mergeLoggingConfig),
		reflect.TypeOf(&types.UlimitsConfig{}):           safelyMerge(mergeUlimitsConfig),
		reflect.TypeOf([]types.ServiceVolumeConfig{}):    mergeSlice(toServiceVolumeConfigsMap, toServiceVolumeConfigsSlice),
		reflect.TypeOf([]types.ServicePortConfig{}):      mergeSlice(toServicePortConfigsMap, toServicePortConfigsSlice),
		reflect.TypeOf([]types.ServiceSecretConfig{}):    mergeSlice(toServiceSecretConfigsMap, toServiceSecretConfigsSlice),
		reflect.TypeOf([]types.ServiceConfigObjConfig{}): mergeSlice(toServiceConfigObjConfigsMap, toSServiceConfigObjConfigsSlice),
		reflect.TypeOf(&types.UlimitsConfig{}):           mergeUlimitsConfig,
		reflect.TypeOf(&types.ServiceNetworkConfig{}):    mergeServiceNetworkConfig,
	},
}

func (s *specials) Transformer(t reflect.Type) func(dst, src reflect.Value) error {
	if fn, ok := s.m[t]; ok {
		return fn
	}
	return nil
}

func merge(configs []*types.Config) (*types.Config, error) {
	base := configs[0]
	for _, override := range configs[1:] {
		var err error
		base.Name = mergeNames(base.Name, override.Name)
		base.Services, err = mergeServices(base.Services, override.Services)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge services from %s", override.Filename)
		}
		base.Volumes, err = mergeVolumes(base.Volumes, override.Volumes)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge volumes from %s", override.Filename)
		}
		base.Networks, err = mergeNetworks(base.Networks, override.Networks)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge networks from %s", override.Filename)
		}
		base.Secrets, err = mergeSecrets(base.Secrets, override.Secrets)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge secrets from %s", override.Filename)
		}
		base.Configs, err = mergeConfigs(base.Configs, override.Configs)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge configs from %s", override.Filename)
		}
		base.Extensions, err = mergeExtensions(base.Extensions, override.Extensions)
		if err != nil {
			return base, errors.Wrapf(err, "cannot merge extensions from %s", override.Filename)
		}
	}
	return base, nil
}

func mergeNames(base, override string) string {
	if override != "" {
		return override
	}
	return base
}

func mergeServices(base, override []types.ServiceConfig) ([]types.ServiceConfig, error) {
	baseServices := mapByName(base)
	overrideServices := mapByName(override)
	for name, overrideService := range overrideServices {
		overrideService := overrideService
		if baseService, ok := baseServices[name]; ok {
			merged, err := _merge(&baseService, &overrideService)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot merge service %s", name)
			}
			baseServices[name] = *merged
			continue
		}
		baseServices[name] = overrideService
	}
	services := []types.ServiceConfig{}
	for _, baseService := range baseServices {
		services = append(services, baseService)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, nil
}

func _merge(baseService *types.ServiceConfig, overrideService *types.ServiceConfig) (*types.ServiceConfig, error) {
	if err := mergo.Merge(baseService, overrideService, mergo.WithAppendSlice, mergo.WithOverride, mergo.WithTransformers(serviceSpecials)); err != nil {
		return nil, err
	}
	if overrideService.Command != nil {
		baseService.Command = overrideService.Command
	}
	if overrideService.Entrypoint != nil {
		baseService.Entrypoint = overrideService.Entrypoint
	}
	if baseService.Environment != nil {
		baseService.Environment.OverrideBy(overrideService.Environment)
	} else {
		baseService.Environment = overrideService.Environment
	}
	return baseService, nil
}

func toServiceSecretConfigsMap(s interface{}) (map[interface{}]interface{}, error) {
	secrets, ok := s.([]types.ServiceSecretConfig)
	if !ok {
		return nil, errors.Errorf("not a serviceSecretConfig: %v", s)
	}
	m := map[interface{}]interface{}{}
	for _, secret := range secrets {
		m[secret.Source] = secret
	}
	return m, nil
}

func toServiceConfigObjConfigsMap(s interface{}) (map[interface{}]interface{}, error) {
	secrets, ok := s.([]types.ServiceConfigObjConfig)
	if !ok {
		return nil, errors.Errorf("not a serviceSecretConfig: %v", s)
	}
	m := map[interface{}]interface{}{}
	for _, secret := range secrets {
		m[secret.Source] = secret
	}
	return m, nil
}

func toServicePortConfigsMap(s interface{}) (map[interface{}]interface{}, error) {
	ports, ok := s.([]types.ServicePortConfig)
	if !ok {
		return nil, errors.Errorf("not a servicePortConfig slice: %v", s)
	}
	m := map[interface{}]interface{}{}
	type port struct {
		target    uint32
		published string
		ip        string
		protocol  string
	}

	for _, p := range ports {
		mergeKey := port{
			target:    p.Target,
			published: p.Published,
			ip:        p.HostIP,
			protocol:  p.Protocol,
		}
		m[mergeKey] = p
	}
	return m, nil
}

func toServiceVolumeConfigsMap(s interface{}) (map[interface{}]interface{}, error) {
	volumes, ok := s.([]types.ServiceVolumeConfig)
	if !ok {
		return nil, errors.Errorf("not a ServiceVolumeConfig slice: %v", s)
	}
	m := map[interface{}]interface{}{}
	for _, v := range volumes {
		m[v.Target] = v
	}
	return m, nil
}

func toServiceSecretConfigsSlice(dst reflect.Value, m map[interface{}]interface{}) error {
	var s []types.ServiceSecretConfig
	for _, v := range m {
		s = append(s, v.(types.ServiceSecretConfig))
	}
	sort.Slice(s, func(i, j int) bool { return s[i].Source < s[j].Source })
	dst.Set(reflect.ValueOf(s))
	return nil
}

func toSServiceConfigObjConfigsSlice(dst reflect.Value, m map[interface{}]interface{}) error {
	var s []types.ServiceConfigObjConfig
	for _, v := range m {
		s = append(s, v.(types.ServiceConfigObjConfig))
	}
	sort.Slice(s, func(i, j int) bool { return s[i].Source < s[j].Source })
	dst.Set(reflect.ValueOf(s))
	return nil
}

func toServicePortConfigsSlice(dst reflect.Value, m map[interface{}]interface{}) error {
	var s []types.ServicePortConfig
	for _, v := range m {
		s = append(s, v.(types.ServicePortConfig))
	}
	sort.Slice(s, func(i, j int) bool {
		if s[i].Target != s[j].Target {
			return s[i].Target < s[j].Target
		}
		if s[i].Published != s[j].Published {
			return s[i].Published < s[j].Published
		}
		if s[i].HostIP != s[j].HostIP {
			return s[i].HostIP < s[j].HostIP
		}
		return s[i].Protocol < s[j].Protocol
	})
	dst.Set(reflect.ValueOf(s))
	return nil
}

func toServiceVolumeConfigsSlice(dst reflect.Value, m map[interface{}]interface{}) error {
	var s []types.ServiceVolumeConfig
	for _, v := range m {
		s = append(s, v.(types.ServiceVolumeConfig))
	}
	sort.Slice(s, func(i, j int) bool { return s[i].Target < s[j].Target })
	dst.Set(reflect.ValueOf(s))
	return nil
}

type toMapFn func(s interface{}) (map[interface{}]interface{}, error)
type writeValueFromMapFn func(reflect.Value, map[interface{}]interface{}) error

func safelyMerge(mergeFn func(dst, src reflect.Value) error) func(dst, src reflect.Value) error {
	return func(dst, src reflect.Value) error {
		if src.IsNil() {
			return nil
		}
		if dst.IsNil() {
			dst.Set(src)
			return nil
		}
		return mergeFn(dst, src)
	}
}

func mergeSlice(toMap toMapFn, writeValue writeValueFromMapFn) func(dst, src reflect.Value) error {
	return func(dst, src reflect.Value) error {
		dstMap, err := sliceToMap(toMap, dst)
		if err != nil {
			return err
		}
		srcMap, err := sliceToMap(toMap, src)
		if err != nil {
			return err
		}
		if err := mergo.Map(&dstMap, srcMap, mergo.WithOverride); err != nil {
			return err
		}
		return writeValue(dst, dstMap)
	}
}

func sliceToMap(toMap toMapFn, v reflect.Value) (map[interface{}]interface{}, error) {
	// check if valid
	if !v.IsValid() {
		return nil, errors.Errorf("invalid value : %+v", v)
	}
	return toMap(v.Interface())
}

func mergeLoggingConfig(dst, src reflect.Value) error {
	// Same driver, merging options
	if getLoggingDriver(dst.Elem()) == getLoggingDriver(src.Elem()) ||
		getLoggingDriver(dst.Elem()) == "" || getLoggingDriver(src.Elem()) == "" {
		if getLoggingDriver(dst.Elem()) == "" {
			dst.Elem().FieldByName("Driver").SetString(getLoggingDriver(src.Elem()))
		}
		dstOptions := dst.Elem().FieldByName("Options").Interface().(map[string]string)
		srcOptions := src.Elem().FieldByName("Options").Interface().(map[string]string)
		return mergo.Merge(&dstOptions, srcOptions, mergo.WithOverride)
	}
	// Different driver, override with src
	dst.Set(src)
	return nil
}

// nolint: unparam
func mergeUlimitsConfig(dst, src reflect.Value) error {
	if src.Interface() != reflect.Zero(reflect.TypeOf(src.Interface())).Interface() {
		dst.Elem().Set(src.Elem())
	}
	return nil
}

// nolint: unparam
func mergeServiceNetworkConfig(dst, src reflect.Value) error {
	if src.Interface() != reflect.Zero(reflect.TypeOf(src.Interface())).Interface() {
		dst.Elem().FieldByName("Aliases").Set(src.Elem().FieldByName("Aliases"))
		if ipv4 := src.Elem().FieldByName("Ipv4Address").Interface().(string); ipv4 != "" {
			dst.Elem().FieldByName("Ipv4Address").SetString(ipv4)
		}
		if ipv6 := src.Elem().FieldByName("Ipv6Address").Interface().(string); ipv6 != "" {
			dst.Elem().FieldByName("Ipv6Address").SetString(ipv6)
		}
	}
	return nil
}

func getLoggingDriver(v reflect.Value) string {
	return v.FieldByName("Driver").String()
}

func mapByName(services []types.ServiceConfig) map[string]types.ServiceConfig {
	m := map[string]types.ServiceConfig{}
	for _, service := range services {
		m[service.Name] = service
	}
	return m
}

func mergeVolumes(base, override map[string]types.VolumeConfig) (map[string]types.VolumeConfig, error) {
	err := mergo.Map(&base, &override, mergo.WithOverride)
	return base, err
}

func mergeNetworks(base, override map[string]types.NetworkConfig) (map[string]types.NetworkConfig, error) {
	err := mergo.Map(&base, &override, mergo.WithOverride)
	return base, err
}

func mergeSecrets(base, override map[string]types.SecretConfig) (map[string]types.SecretConfig, error) {
	err := mergo.Map(&base, &override, mergo.WithOverride)
	return base, err
}

func mergeConfigs(base, override map[string]types.ConfigObjConfig) (map[string]types.ConfigObjConfig, error) {
	err := mergo.Map(&base, &override, mergo.WithOverride)
	return base, err
}

func mergeExtensions(base, override map[string]interface{}) (map[string]interface{}, error) {
	if base == nil {
		base = map[string]interface{}{}
	}
	err := mergo.Map(&base, &override, mergo.WithOverride)
	return base, err
}
