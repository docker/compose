/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package client

import (
	"context"

	"github.com/docker/api/context/cloud"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

// New returns a backend client
func New(ctx context.Context) (*Client, error) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	cc, err := s.Get(currentContext)
	if err != nil {
		return nil, err
	}

	service, err := backend.Get(ctx, cc.Type)
	if err != nil {
		return nil, err
	}

	return &Client{
		backendType: cc.Type,
		bs:          service,
	}, nil

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

// CloudService returns the backend service for the current context
func (c *Client) CloudService() cloud.Service {
	return c.bs.CloudService()
}
