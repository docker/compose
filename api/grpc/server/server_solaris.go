package server

import (
	"errors"

	"github.com/docker/containerd/api/grpc/types"
	"golang.org/x/net/context"
)

var clockTicksPerSecond uint64

func (s *apiServer) AddProcess(ctx context.Context, r *types.AddProcessRequest) (*types.AddProcessResponse, error) {
	return &types.AddProcessResponse{}, errors.New("apiServer AddProcess() not implemented on Solaris")
}
