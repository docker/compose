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
	"errors"

	"github.com/docker/api/backend"
	backendv1 "github.com/docker/api/backend/v1"
	cliv1 "github.com/docker/api/cli/v1"
	composev1 "github.com/docker/api/compose/v1"
	"github.com/docker/api/containers"
	containersv1 "github.com/docker/api/containers/v1"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

// New returns a backend client
func New(ctx context.Context) (*Client, error) {
	currentContext := apicontext.CurrentContext(ctx)
	s := store.ContextStore(ctx)

	cc, err := s.Get(currentContext, nil)
	if err != nil {
		return nil, err
	}
	contextType := s.GetType(cc)

	b, err := backend.Get(ctx, contextType)
	if err != nil {
		return nil, err
	}

	ba, ok := b.(containers.ContainerService)
	if !ok {
		return nil, errors.New("backend not found")
	}
	return &Client{
		backendType: contextType,
		cc:          ba,
	}, nil

}

// Client is a multi-backend client
type Client struct {
	backendv1.BackendClient
	cliv1.CliClient
	containersv1.ContainersClient
	composev1.ComposeClient

	backendType string
	cc          containers.ContainerService
}

// ContainerService returns the backend service for the current context
func (c *Client) ContainerService() containers.ContainerService {
	return c.cc
}
