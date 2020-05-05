package backend

import (
	"context"
	"errors"
	"fmt"
)

var (
	errNoType         = errors.New("backend: no type")
	errNoName         = errors.New("backend: no name")
	errTypeRegistered = errors.New("backend: already registered")
)

type initFunc func(context.Context) (interface{}, error)

type registeredBackend struct {
	name        string
	backendType string
	init        initFunc
}

var backends = struct {
	r []*registeredBackend
}{}

// Register adds a typed backend to the registry
func Register(name string, backendType string, init initFunc) {
	if name == "" {
		panic(errNoName)
	}
	if backendType == "" {
		panic(errNoType)
	}
	for _, b := range backends.r {
		if b.backendType == backendType {
			panic(errTypeRegistered)
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
func Get(ctx context.Context, backendType string) (interface{}, error) {
	for _, b := range backends.r {
		if b.backendType == backendType {
			return b.init(ctx)
		}
	}

	return nil, fmt.Errorf("backend not found for context %q", backendType)
}
