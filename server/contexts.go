package server

import (
	"context"

	"github.com/docker/api/context/store"
	contextsv1 "github.com/docker/api/protos/contexts/v1"
)

type cliServer struct {
}

// NewContexts returns a contexts server
func NewContexts() contextsv1.ContextsServer {
	return &cliServer{}
}

func (cs *cliServer) List(ctx context.Context, request *contextsv1.ListRequest) (*contextsv1.ListResponse, error) {
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		return &contextsv1.ListResponse{}, err
	}

	result := &contextsv1.ListResponse{}

	for _, c := range contexts {
		result.Contexts = append(result.Contexts, &contextsv1.Context{
			Name:        c.Name,
			ContextType: c.Type,
		})
	}

	return result, nil
}
