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

package proxy

import (
	"context"
	"sync"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/config"
	containersv1 "github.com/docker/compose-cli/cli/server/protos/containers/v1"
	contextsv1 "github.com/docker/compose-cli/cli/server/protos/contexts/v1"
	streamsv1 "github.com/docker/compose-cli/cli/server/protos/streams/v1"
	volumesv1 "github.com/docker/compose-cli/cli/server/protos/volumes/v1"
	"github.com/docker/compose-cli/cli/server/proxy/streams"
)

type clientKey struct{}

// WithClient adds the client to the context
func WithClient(ctx context.Context, c *client.Client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

// Client returns the client from the context
func Client(ctx context.Context) *client.Client {
	c, _ := ctx.Value(clientKey{}).(*client.Client)
	return c
}

// Proxy implements the gRPC server and forwards the actions
// to the right backend
type Proxy interface {
	containersv1.ContainersServer
	streamsv1.StreamingServer
	volumesv1.VolumesServer
	ContextsProxy() contextsv1.ContextsServer
}

type proxy struct {
	configDir     string
	mu            sync.Mutex
	streams       map[string]*streams.Stream
	contextsProxy *contextsProxy
}

// New creates a new proxy server
func New(ctx context.Context) Proxy {
	configDir := config.Dir()
	return &proxy{
		configDir: configDir,
		streams:   map[string]*streams.Stream{},
		contextsProxy: &contextsProxy{
			configDir: configDir,
		},
	}
}

func (p *proxy) ContextsProxy() contextsv1.ContextsServer {
	return p.contextsProxy
}
