package backend

import (
	"context"
	"errors"
)

var (
	ErrNoType         = errors.New("backend: no type")
	ErrNoName         = errors.New("backend: no name")
	ErrTypeRegistered = errors.New("backend: already registered")
)

type InitFunc func(context.Context) (interface{}, error)

type Backend struct {
	name        string
	backendType string
	init        InitFunc
}

var backends = struct {
	r []*Backend
}{}

func Register(name string, backendType string, init InitFunc) {
	if name == "" {
		panic(ErrNoName)
	}
	if backendType == "" {
		panic(ErrNoType)
	}
	for _, b := range backends.r {
		if b.backendType == backendType {
			panic(ErrTypeRegistered)
		}
	}

	backends.r = append(backends.r, &Backend{
		name,
		backendType,
		init,
	})
}

func Get(ctx context.Context, backendType string) (interface{}, error) {
	for _, b := range backends.r {
		if b.backendType == backendType {
			return b.init(ctx)
		}
	}

	return nil, errors.New("not found")
}
