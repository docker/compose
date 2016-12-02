package execution

import (
	"context"

	"github.com/docker/containerd"
	api "github.com/docker/containerd/api/execution"
	"github.com/docker/containerd/executors"
)

type Opts struct {
	Root    string
	Runtime string
}

func New(o Opts) (*Service, error) {
	executor, err := executors.Get(o.Runtime)(o.Root)
	if err != nil {
		return nil, err
	}
	return &Service{
		o:        o,
		executor: executor,
	}, nil
}

type Service struct {
	o        Opts
	executor containerd.Executor
}

func (s *Service) Create(ctx context.Context, r *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	// TODO: write io and bundle path to dir
	container, err := s.executor.Create(r.ID, r.BundlePath, &IO{})
	if err != nil {
		return nil, err
	}

	s.supervisor.Add(container.Process())

	return &api.CreateContainerResponse{
		Container: toGRPCContainer(container),
	}, nil
}

func (s *Service) Delete(ctx context.Context, r *api.DeleteContainerRequest) (*api.Empty, error) {
	if err := s.executor.Delete(r.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) List(ctx context.Context, r *api.ListContainerRequest) (*api.ListContainerResponse, error) {
	containers, err := s.executor.List()
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		r.Containers = append(r.Containers, toGRPCContainer(c))
	}
	return r, nil
}

var (
	_ = (api.ExecutionServiceServer)(&Service{})
	_ = (api.ContainerServiceServer)(&Service{})
)
