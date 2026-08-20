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
	"fmt"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/override"
	"github.com/compose-spec/compose-go/v2/types"
	yaml "go.yaml.in/yaml/v4"
)

// mergedPreStartSpec merges the hook's container specification over the
// service's, following the compose file merge rules: command and entrypoint
// replace, environment merges per key with the hook winning, extra_hosts and
// dns accumulate entries, ulimits merge, ... Every ContainerSpec attribute
// inherits this way, current and future, without attribute-specific code.
//
// Volumes are deliberately dropped from the inherited side: mounts inherit
// at runtime through volumes_from, the only mechanism that shares the
// service's anonymous and image volumes; the merged spec carries the hook's
// own volume declarations only, which take precedence per target.
func mergedPreStartSpec(service types.ServiceConfig, hook types.PreStartHook) (types.ContainerSpec, error) {
	base, err := containerSpecDict(service.ContainerSpec)
	if err != nil {
		return types.ContainerSpec{}, err
	}
	delete(base, "volumes")
	over, err := containerSpecDict(hook.ContainerSpec)
	if err != nil {
		return types.ContainerSpec{}, err
	}

	merged, err := override.Merge(
		map[string]any{"services": map[string]any{"hook": base}},
		map[string]any{"services": map[string]any{"hook": over}},
	)
	if err != nil {
		return types.ContainerSpec{}, fmt.Errorf("merging pre_start hook specification: %w", err)
	}
	dict, ok := merged["services"].(map[string]any)["hook"].(map[string]any)
	if !ok {
		return types.ContainerSpec{}, fmt.Errorf("internal: unexpected merged hook specification shape")
	}

	var spec types.ContainerSpec
	if err := loader.Transform(dict, &spec); err != nil {
		return types.ContainerSpec{}, fmt.Errorf("decoding merged pre_start hook specification: %w", err)
	}
	return spec, nil
}

// containerSpecDict serializes a ContainerSpec to the canonical yaml tree the
// compose merge rules operate on — the same serialization compose config
// uses.
func containerSpecDict(spec types.ContainerSpec) (map[string]any, error) {
	p := &types.Project{Services: types.Services{"hook": {ContainerSpec: spec}}}
	raw, err := p.MarshalYAML()
	if err != nil {
		return nil, err
	}
	var dict map[string]any
	if err := yaml.Unmarshal(raw, &dict); err != nil {
		return nil, err
	}
	svc, ok := dict["services"].(map[string]any)["hook"].(map[string]any)
	if !ok {
		// a zero spec marshals to a null service entry
		return map[string]any{}, nil
	}
	delete(svc, "name")
	return svc, nil
}
