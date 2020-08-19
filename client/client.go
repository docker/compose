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

package client

import (
	"context"

	"github.com/docker/api/secrets"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/cloud"
	"github.com/docker/api/context/store"
)

// New returns a backend client associated with current context
func New(ctx context.Context) (*Client, error) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	cc, err := s.Get(currentContext)
	if err != nil {
		return nil, err
	}

	service, err := backend.Get(ctx, cc.Type())
	if err != nil {
		return nil, err
	}

	return &Client{
		backendType: cc.Type(),
		bs:          service,
	}, nil
}

// GetCloudService returns a backend CloudService (typically login, create context)
func GetCloudService(ctx context.Context, backendType string) (cloud.Service, error) {
	return backend.GetCloudService(ctx, backendType)
}

// Client is a multi-backend client
type Client struct {
	backendType string
	bs          backend.Service
}

// ContainerService returns the backend service for the current context
func (c *Client) ContainerService() containers.Service {
	return c.bs.ContainerService()
}

// ComposeService returns the backend service for the current context
func (c *Client) ComposeService() compose.Service {
	return c.bs.ComposeService()
}

// SecretsService returns the backend service for the current context
func (c *Client) SecretsService() secrets.Service {
	return c.bs.SecretsService()
}
