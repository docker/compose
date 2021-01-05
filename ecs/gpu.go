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

package ecs

import (
	"fmt"
	"math"
	"strconv"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/go-units"
)

type machine struct {
	id     string
	cpus   float64
	memory types.UnitBytes
	gpus   int64
}

type family []machine

var gpufamily = family{
	{
		id:     "g4dn.xlarge",
		cpus:   4,
		memory: 16 * units.GiB,
		gpus:   1,
	},
	{
		id:     "g4dn.2xlarge",
		cpus:   8,
		memory: 32 * units.GiB,
		gpus:   1,
	},
	{
		id:     "g4dn.4xlarge",
		cpus:   16,
		memory: 64 * units.GiB,
		gpus:   1,
	},
	{
		id:     "g4dn.8xlarge",
		cpus:   32,
		memory: 128 * units.GiB,
		gpus:   1,
	},
	{
		id:     "g4dn.12xlarge",
		cpus:   48,
		memory: 192 * units.GiB,
		gpus:   4,
	},
	{
		id:     "g4dn.16xlarge",
		cpus:   64,
		memory: 256 * units.GiB,
		gpus:   1,
	},
	{
		id:     "g4dn.metal",
		cpus:   96,
		memory: 384 * units.GiB,
		gpus:   8,
	},
}

type filterFn func(machine) bool

func (f family) filter(fn filterFn) family {
	var filtered family
	for _, machine := range f {
		if fn(machine) {
			filtered = append(filtered, machine)
		}
	}
	return filtered
}

func (f family) firstOrError(msg string, args ...interface{}) (machine, error) {
	if len(f) == 0 {
		return machine{}, fmt.Errorf(msg, args...)
	}
	return f[0], nil
}

func guessMachineType(project *types.Project) (string, error) {
	// we select a machine type to match all gpus-bound services requirements
	// once https://github.com/aws/containers-roadmap/issues/631 is implemented we can define dedicated CapacityProviders per service.
	requirements, err := getResourceRequirements(project)
	if err != nil {
		return "", err
	}

	instanceType, err := gpufamily.
		filter(func(m machine) bool {
			return m.memory > requirements.memory // actual memory available for ECS tasks < total machine memory
		}).
		filter(func(m machine) bool {
			return m.cpus >= requirements.cpus
		}).
		filter(func(m machine) bool {
			return m.gpus >= requirements.gpus
		}).
		firstOrError("none of the Amazon EC2 G4 instance types meet the requirements for memory:%d cpu:%f gpus:%d", requirements.memory, requirements.cpus, requirements.gpus)
	if err != nil {
		return "", err
	}
	return instanceType.id, nil
}

type resourceRequirements struct {
	memory types.UnitBytes
	cpus   float64
	gpus   int64
}

func getResourceRequirements(project *types.Project) (*resourceRequirements, error) {
	return toResourceRequirementsSlice(project).
		filter(func(requirements *resourceRequirements) bool {
			return requirements != nil && requirements.gpus != 0
		}).
		max()
}

type eitherRequirementsOrError struct {
	requirements []*resourceRequirements
	err          error
}

func toResourceRequirementsSlice(project *types.Project) eitherRequirementsOrError {
	var requirements []*resourceRequirements
	for _, service := range project.Services {
		r, err := toResourceRequirements(service)
		if err != nil {
			return eitherRequirementsOrError{nil, err}
		}
		requirements = append(requirements, r)
	}
	return eitherRequirementsOrError{requirements, nil}
}

func (r eitherRequirementsOrError) filter(fn func(*resourceRequirements) bool) eitherRequirementsOrError {
	if r.err != nil {
		return r
	}
	var requirements []*resourceRequirements
	for _, req := range r.requirements {
		if fn(req) {
			requirements = append(requirements, req)
		}
	}
	return eitherRequirementsOrError{requirements, nil}
}

func toResourceRequirements(service types.ServiceConfig) (*resourceRequirements, error) {
	if service.Deploy == nil {
		return nil, nil
	}
	reservations := service.Deploy.Resources.Reservations
	if reservations == nil {
		return nil, nil
	}

	var requiredGPUs int64
	for _, r := range reservations.GenericResources {
		if r.DiscreteResourceSpec.Kind == "gpus" {
			requiredGPUs = r.DiscreteResourceSpec.Value
			break
		}
	}

	for _, r := range reservations.Devices {
		requiresGpus := false
		for _, c := range r.Capabilities {
			if c == "gpu" {
				requiresGpus = true
				break
			}
		}
		if requiresGpus {
			requiredGPUs = r.Count
			if requiredGPUs <= 0 {
				requiredGPUs = 1
			}
			break
		}
	}

	var nanocpu float64
	if reservations.NanoCPUs != "" {
		v, err := strconv.ParseFloat(reservations.NanoCPUs, 64)
		if err != nil {
			return nil, err
		}
		nanocpu = v
	}
	return &resourceRequirements{
		memory: reservations.MemoryBytes,
		cpus:   nanocpu,
		gpus:   requiredGPUs,
	}, nil
}

func (r resourceRequirements) combine(o *resourceRequirements) resourceRequirements {
	if o == nil {
		return r
	}
	return resourceRequirements{
		memory: maxUnitBytes(r.memory, o.memory),
		cpus:   math.Max(r.cpus, o.cpus),
		gpus:   maxInt64(r.gpus, o.gpus),
	}
}

func (r eitherRequirementsOrError) max() (*resourceRequirements, error) {
	if r.err != nil {
		return nil, r.err
	}
	min := resourceRequirements{}
	for _, req := range r.requirements {
		min = min.combine(req)
	}
	return &min, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxUnitBytes(a, b types.UnitBytes) types.UnitBytes {
	if a > b {
		return a
	}
	return b
}
