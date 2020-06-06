package proxy

import (
	"context"
	"sync"

	"github.com/docker/api/client"
	containersv1 "github.com/docker/api/protos/containers/v1"
	contextsv1 "github.com/docker/api/protos/contexts/v1"
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
	ContextsProxy() contextsv1.ContextsServer
}

type proxy struct {
	currentContext string
	mu             sync.Mutex
	streams        map[string]*Stream
	contextsProxy  *contextsProxy
}

// New creates a new proxy server
func New(currentContext string) Proxy {
	return &proxy{
		currentContext: currentContext,
		streams:        map[string]*Stream{},
		contextsProxy:  &contextsProxy{},
	}
}

func (p *proxy) ContextsProxy() contextsv1.ContextsServer {
	return p.contextsProxy
}
