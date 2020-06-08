package proxy

import (
	"context"

	"github.com/docker/api/config"
	"github.com/docker/api/context/store"
	contextsv1 "github.com/docker/api/protos/contexts/v1"
)

type contextsProxy struct {
	configDir string
}

func (cp *contextsProxy) SetCurrent(ctx context.Context, request *contextsv1.SetCurrentRequest) (*contextsv1.SetCurrentResponse, error) {
	if err := config.WriteCurrentContext(cp.configDir, request.GetName()); err != nil {
		return &contextsv1.SetCurrentResponse{}, err
	}

	return &contextsv1.SetCurrentResponse{}, nil
}

func (cp *contextsProxy) List(ctx context.Context, request *contextsv1.ListRequest) (*contextsv1.ListResponse, error) {
	s := store.ContextStore(ctx)
	configFile, err := config.LoadFile(cp.configDir)
	if err != nil {
		return nil, err
	}
	contexts, err := s.List()
	if err != nil {
		return &contextsv1.ListResponse{}, err
	}

	result := &contextsv1.ListResponse{}

	for _, c := range contexts {
		result.Contexts = append(result.Contexts, &contextsv1.Context{
			Name:        c.Name,
			ContextType: c.Type,
			Current:     c.Name == configFile.CurrentContext,
		})
	}

	return result, nil
}
