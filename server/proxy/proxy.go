package proxy

import (
	"context"
	"sync"

	"github.com/docker/api/client"
	containersv1 "github.com/docker/api/protos/containers/v1"
	streamsv1 "github.com/docker/api/protos/streams/v1"
)

type clientKey struct{}

// WithClient adds the client to the context
func WithClient(ctx context.Context, c *client.Client) (context.Context, error) {
	return context.WithValue(ctx, clientKey{}, c), nil
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
}

type proxy struct {
	mu      sync.Mutex
	streams map[string]*Stream
}

// New creates a new proxy server
func New() Proxy {
	return &proxy{
		streams: map[string]*Stream{},
	}
}
