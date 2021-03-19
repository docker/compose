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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/pkg/errors"
)

// ContainersKey is the context key to access context value os a ContainersStatus
type ContainersKey struct{}

// ContainersState state management interface
type ContainersState interface {
	Get(string) *types.Container
	GetContainers() Containers
	Add(c types.Container)
	AddAll(cs Containers)
	Remove(string) types.Container
}

// NewContainersState creates a new container state manager
func NewContainersState(cs Containers) ContainersState {
	s := containersState{
		observedContainers: &cs,
	}
	return &s
}

// ContainersStatus works as a collection container for the observed containers
type containersState struct {
	observedContainers *Containers
}

func (s *containersState) AddAll(cs Containers) {
	for _, c := range cs {
		lValue := append(*s.observedContainers, c)
		s.observedContainers = &lValue
	}
}

func (s *containersState) Add(c types.Container) {
	if s.Get(c.ID) == nil {
		lValue := append(*s.observedContainers, c)
		s.observedContainers = &lValue
	}
}

func (s *containersState) Remove(id string) types.Container {
	var c types.Container
	var newObserved Containers
	for _, o := range *s.observedContainers {
		if o.ID != id {
			c = o
			continue
		}
		newObserved = append(newObserved, o)
	}
	s.observedContainers = &newObserved
	return c
}

func (s *containersState) Get(id string) *types.Container {
	for _, o := range *s.observedContainers {
		if id == o.ID {
			return &o
		}
	}
	return nil
}

func (s *containersState) GetContainers() Containers {
	if s.observedContainers != nil && *s.observedContainers != nil {
		return *s.observedContainers
	}
	return make(Containers, 0)
}

// GetContextContainerState gets the container state manager
func GetContextContainerState(ctx context.Context) (ContainersState, error) {
	cState, ok := ctx.Value(ContainersKey{}).(*containersState)
	if !ok {
		return nil, errors.New("containers' containersState not available in context")
	}
	return cState, nil
}

func (s composeService) getUpdatedContainersStateContext(ctx context.Context, projectName string) (context.Context, error) {
	observedState, err := s.apiClient.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
		),
		All: true,
	})
	if err != nil {
		return nil, err
	}
	containerState := NewContainersState(observedState)
	return context.WithValue(ctx, ContainersKey{}, containerState), nil
}
