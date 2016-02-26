package server

import (
	"errors"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/supervisor"
	"golang.org/x/net/context"
)

// noop on Windows (Checkpoints not supported)
func createContainerConfigCheckpoint(e *supervisor.StartTask, c *types.CreateContainerRequest) {
}

// TODO Windows - may be able to completely factor out
func (s *apiServer) CreateCheckpoint(ctx context.Context, r *types.CreateCheckpointRequest) (*types.CreateCheckpointResponse, error) {
	return nil, errors.New("CreateCheckpoint() not supported on Windows")
}

// TODO Windows - may be able to completely factor out
func (s *apiServer) DeleteCheckpoint(ctx context.Context, r *types.DeleteCheckpointRequest) (*types.DeleteCheckpointResponse, error) {
	return nil, errors.New("DeleteCheckpoint() not supported on Windows")
}

// TODO Windows - may be able to completely factor out
func (s *apiServer) ListCheckpoint(ctx context.Context, r *types.ListCheckpointRequest) (*types.ListCheckpointResponse, error) {
	return nil, errors.New("ListCheckpoint() not supported on Windows")
}

func (s *apiServer) Stats(ctx context.Context, r *types.StatsRequest) (*types.StatsResponse, error) {
	return nil, errors.New("Stats() not supported on Windows")
}

func setUserFieldsInProcess(p *types.Process, oldProc runtime.ProcessSpec) {
}

func setPlatformRuntimeProcessSpecUserFields(r *types.User, process *runtime.ProcessSpec) {
}
