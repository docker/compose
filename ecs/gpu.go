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

package ecs

import (
	"fmt"
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

var p3family = family{
	{
		id:     "p3.2xlarge",
		cpus:   8,
		memory: 64 * units.GiB,
		gpus:   2,
	},
	{
		id:     "p3.8xlarge",
		cpus:   32,
		memory: 244 * units.GiB,
		gpus:   4,
	},
	{
		id:     "p3.16xlarge",
		cpus:   64,
		memory: 488 * units.GiB,
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
	// we select a machine type to match all gpu-bound services requirements
	// once https://github.com/aws/containers-roadmap/issues/631 is implemented we can define dedicated CapacityProviders per service.
	minMemory, minCPU, minGPU, err := getResourceRequirements(project)
	if err != nil {
		return "", err
	}

	instanceType, err := p3family.
		filter(func(m machine) bool {
			return m.memory >= minMemory
		}).
		filter(func(m machine) bool {
			return m.cpus >= minCPU
		}).
		filter(func(m machine) bool {
			return m.gpus >= minGPU
		}).
		firstOrError("none of the AWS p3 machines match requirement for memory:%d cpu:%f gpu:%d", minMemory, minCPU, minGPU)
	if err != nil {
		return "", err
	}
	return instanceType.id, nil
}

func getResourceRequirements(project *types.Project) (types.UnitBytes, float64, int64, error) {
	var minMemory types.UnitBytes
	var minCPU float64
	var minGPU int64
	for _, service := range project.Services {
		if service.Deploy == nil {
			continue
		}
		reservations := service.Deploy.Resources.Reservations
		if reservations == nil {
			continue
		}

		var requiredGPUs int64
		for _, r := range reservations.GenericResources {
			if r.DiscreteResourceSpec.Kind == "gpu" {
				requiredGPUs = r.DiscreteResourceSpec.Value
				break
			}
		}
		if requiredGPUs == 0 {
			continue
		}
		if requiredGPUs > minGPU {
			minGPU = requiredGPUs
		}

		if reservations.MemoryBytes > minMemory {
			minMemory = reservations.MemoryBytes
		}
		if reservations.NanoCPUs != "" {
			nanocpu, err := strconv.ParseFloat(reservations.NanoCPUs, 64)
			if err != nil {
				return 0, 0, 0, err
			}
			if nanocpu > minCPU {
				minCPU = nanocpu
			}
		}
	}
	return minMemory, minCPU, minGPU, nil
}
