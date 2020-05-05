package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
)

var (
	errNoType         = errors.New("backend: no type")
	errNoName         = errors.New("backend: no name")
	errTypeRegistered = errors.New("backend: already registered")
)

type initFunc func(context.Context) (Service, error)

type registeredBackend struct {
	name        string
	backendType string
	init        initFunc
}

var backends = struct {
	r []*registeredBackend
}{}

// Service aggregates the service interfaces
type Service interface {
	ContainerService() containers.Service
	ComposeService() compose.Service
}

// Register adds a typed backend to the registry
func Register(name string, backendType string, init initFunc) {
	if name == "" {
		logrus.Fatal(errNoName)
	}
	if backendType == "" {
		logrus.Fatal(errNoType)
	}
	for _, b := range backends.r {
		if b.backendType == backendType {
			logrus.Fatal(errTypeRegistered)
		}
	}

	backends.r = append(backends.r, &registeredBackend{
		name,
		backendType,
		init,
	})
}

// Get returns the backend registered for a particular type, it returns
// an error if there is no registered backends for the given type.
func Get(ctx context.Context, backendType string) (Service, error) {
	for _, b := range backends.r {
		if b.backendType == backendType {
			return b.init(ctx)
		}
	}

	return nil, fmt.Errorf("backend not found for context %q", backendType)
}
