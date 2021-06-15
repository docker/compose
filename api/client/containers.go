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

package client

import (
	"context"

	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/pkg/api"
)

type containerService struct {
}

// List returns all the containers
func (c *containerService) List(context.Context, bool) ([]containers.Container, error) {
	return nil, api.ErrNotImplemented
}

// Start starts a stopped container
func (c *containerService) Start(context.Context, string) error {
	return api.ErrNotImplemented
}

// Stop stops the running container
func (c *containerService) Stop(context.Context, string, *uint32) error {
	return api.ErrNotImplemented
}

func (c *containerService) Kill(ctx context.Context, containerID string, signal string) error {
	return api.ErrNotImplemented
}

// Run creates and starts a container
func (c *containerService) Run(context.Context, containers.ContainerConfig) error {
	return api.ErrNotImplemented
}

// Exec executes a command inside a running container
func (c *containerService) Exec(context.Context, string, containers.ExecRequest) error {
	return api.ErrNotImplemented
}

// Logs returns all the logs of a container
func (c *containerService) Logs(context.Context, string, containers.LogsRequest) error {
	return api.ErrNotImplemented
}

// Delete removes containers
func (c *containerService) Delete(context.Context, string, containers.DeleteRequest) error {
	return api.ErrNotImplemented
}

// Inspect get a specific container
func (c *containerService) Inspect(context.Context, string) (containers.Container, error) {
	return containers.Container{}, api.ErrNotImplemented
}
