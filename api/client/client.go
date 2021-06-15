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

	"github.com/docker/compose-cli/api/backend"
	"github.com/docker/compose-cli/api/cloud"
	"github.com/docker/compose-cli/api/containers"
	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/pkg/api"
)

// New returns a backend client associated with current context
func New(ctx context.Context) (*Client, error) {
	currentContext := apicontext.Current()
	s := store.Instance()

	cc, err := s.Get(currentContext)
	if err != nil {
		return nil, err
	}

	service := backend.Current()
	if service == nil {
		return nil, api.ErrNotFound
	}

	client := NewClient(cc.Type(), service)
	return &client, nil
}

// NewClient returns new client
func NewClient(backendType string, service backend.Service) Client {
	return Client{
		backendType: backendType,
		bs:          service,
	}
}

// GetCloudService returns a backend CloudService (typically login, create context)
func GetCloudService(ctx context.Context, backendType string) (cloud.Service, error) {
	return backend.GetCloudService(backendType)
}

// Client is a multi-backend client
type Client struct {
	backendType string
	bs          backend.Service
}

// ContextType the context type associated with backend
func (c *Client) ContextType() string {
	return c.backendType
}

// ContainerService returns the backend service for the current context
func (c *Client) ContainerService() containers.Service {
	if cs := c.bs.ContainerService(); cs != nil {
		return cs
	}

	return &containerService{}
}

// ComposeService returns the backend service for the current context
func (c *Client) ComposeService() api.Service {
	if cs := c.bs.ComposeService(); cs != nil {
		return cs
	}

	return &composeService{}
}

// SecretsService returns the backend service for the current context
func (c *Client) SecretsService() secrets.Service {
	if ss := c.bs.SecretsService(); ss != nil {
		return ss
	}

	return &secretsService{}
}

// VolumeService returns the backend service for the current context
func (c *Client) VolumeService() volumes.Service {
	if vs := c.bs.VolumeService(); vs != nil {
		return vs
	}

	return &volumeService{}
}

// ResourceService returns the backend service for the current context
func (c *Client) ResourceService() resources.Service {
	if vs := c.bs.ResourceService(); vs != nil {
		return vs
	}

	return &resourceService{}
}
