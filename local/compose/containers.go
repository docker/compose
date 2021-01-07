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

import moby "github.com/docker/docker/api/types"

// Containers is a set of moby Container
type Containers []moby.Container

// containerPredicate define a predicate we want container to satisfy for filtering operations
type containerPredicate func(c moby.Container) bool

func isService(services ...string) containerPredicate {
	return func(c moby.Container) bool {
		service := c.Labels[serviceLabel]
		return contains(services, service)
	}
}

func isNotService(services ...string) containerPredicate {
	return func(c moby.Container) bool {
		service := c.Labels[serviceLabel]
		return !contains(services, service)
	}
}

// filter return Containers with elements to match predicate
func (containers Containers) filter(predicate containerPredicate) Containers {
	var filtered Containers
	for _, c := range containers {
		if predicate(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// split return Containers with elements to match and those not to match predicate
func (containers Containers) split(predicate containerPredicate) (Containers, Containers) {
	var right Containers
	var left Containers
	for _, c := range containers {
		if predicate(c) {
			right = append(right, c)
		} else {
			left = append(left, c)
		}
	}
	return right, left
}

func (containers Containers) names() []string {
	var names []string
	for _, c := range containers {
		names = append(names, getContainerName(c))
	}
	return names
}
