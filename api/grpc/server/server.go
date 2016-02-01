package server

import (
	"errors"
	"fmt"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/supervisor"
	"github.com/opencontainers/specs"
	"golang.org/x/net/context"
)

type apiServer struct {
	sv *supervisor.Supervisor
}

// NewServer returns grpc server instance
func NewServer(sv *supervisor.Supervisor) types.APIServer {
	return &apiServer{
		sv: sv,
	}
}

func (s *apiServer) CreateContainer(ctx context.Context, c *types.CreateContainerRequest) (*types.CreateContainerResponse, error) {
	if c.BundlePath == "" {
		return nil, errors.New("empty bundle path")
	}
	e := supervisor.NewEvent(supervisor.StartContainerEventType)
	e.ID = c.Id
	e.BundlePath = c.BundlePath
	e.StartResponse = make(chan supervisor.StartResponse, 1)
	if c.Checkpoint != "" {
		e.Checkpoint = &runtime.Checkpoint{
			Name: c.Checkpoint,
		}
	}
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	sr := <-e.StartResponse
	return &types.CreateContainerResponse{
		Stdin:  sr.Stdin,
		Stdout: sr.Stdout,
		Stderr: sr.Stderr,
	}, nil
}

func (s *apiServer) Signal(ctx context.Context, r *types.SignalRequest) (*types.SignalResponse, error) {
	e := supervisor.NewEvent(supervisor.SignalEventType)
	e.ID = r.Id
	e.Pid = r.Pid
	e.Signal = syscall.Signal(int(r.Signal))
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	return &types.SignalResponse{}, nil
}

func (s *apiServer) AddProcess(ctx context.Context, r *types.AddProcessRequest) (*types.AddProcessResponse, error) {
	process := &specs.Process{
		Terminal: r.Terminal,
		Args:     r.Args,
		Env:      r.Env,
		Cwd:      r.Cwd,
		User: specs.User{
			UID:            r.User.Uid,
			GID:            r.User.Gid,
			AdditionalGids: r.User.AdditionalGids,
		},
	}
	if r.Id == "" {
		return nil, fmt.Errorf("container id cannot be empty")
	}
	if r.Pid == "" {
		return nil, fmt.Errorf("process id cannot be empty")
	}
	e := supervisor.NewEvent(supervisor.AddProcessEventType)
	e.ID = r.Id
	e.Pid = r.Pid
	e.ProcessSpec = process
	e.StartResponse = make(chan supervisor.StartResponse, 1)
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	sr := <-e.StartResponse
	return &types.AddProcessResponse{
		Stdin:  sr.Stdin,
		Stdout: sr.Stdout,
		Stderr: sr.Stderr,
	}, nil
}

func (s *apiServer) CreateCheckpoint(ctx context.Context, r *types.CreateCheckpointRequest) (*types.CreateCheckpointResponse, error) {
	e := supervisor.NewEvent(supervisor.CreateCheckpointEventType)
	e.ID = r.Id
	e.Checkpoint = &runtime.Checkpoint{
		Name:        r.Checkpoint.Name,
		Exit:        r.Checkpoint.Exit,
		Tcp:         r.Checkpoint.Tcp,
		UnixSockets: r.Checkpoint.UnixSockets,
		Shell:       r.Checkpoint.Shell,
	}
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	return &types.CreateCheckpointResponse{}, nil
}

func (s *apiServer) DeleteCheckpoint(ctx context.Context, r *types.DeleteCheckpointRequest) (*types.DeleteCheckpointResponse, error) {
	if r.Name == "" {
		return nil, errors.New("checkpoint name cannot be empty")
	}
	e := supervisor.NewEvent(supervisor.DeleteCheckpointEventType)
	e.ID = r.Id
	e.Checkpoint = &runtime.Checkpoint{
		Name: r.Name,
	}
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	return &types.DeleteCheckpointResponse{}, nil
}

func (s *apiServer) ListCheckpoint(ctx context.Context, r *types.ListCheckpointRequest) (*types.ListCheckpointResponse, error) {
	e := supervisor.NewEvent(supervisor.GetContainerEventType)
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	var container runtime.Container
	for _, c := range e.Containers {
		if c.ID() == r.Id {
			container = c
			break
		}
	}
	if container == nil {
		return nil, grpc.Errorf(codes.NotFound, "no such containers")
	}
	var out []*types.Checkpoint
	/*
		checkpoints, err := container.Checkpoints()
		if err != nil {
			return nil, err
		}
		for _, c := range checkpoints {
			out = append(out, &types.Checkpoint{
				Name:        c.Name,
				Tcp:         c.Tcp,
				Shell:       c.Shell,
				UnixSockets: c.UnixSockets,
				// TODO: figure out timestamp
				//Timestamp:   c.Timestamp,
			})
		}
	*/
	return &types.ListCheckpointResponse{Checkpoints: out}, nil
}

func (s *apiServer) State(ctx context.Context, r *types.StateRequest) (*types.StateResponse, error) {
	e := supervisor.NewEvent(supervisor.GetContainerEventType)
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	m := s.sv.Machine()
	state := &types.StateResponse{
		Machine: &types.Machine{
			Cpus:   uint32(m.Cpus),
			Memory: uint64(m.Cpus),
		},
	}
	for _, c := range e.Containers {
		processes, err := c.Processes()
		if err != nil {
			return nil, grpc.Errorf(codes.Internal, "get processes for container")
		}
		var procs []*types.Process
		for _, p := range processes {
			oldProc := p.Spec()
			procs = append(procs, &types.Process{
				Pid:      p.ID(),
				Terminal: oldProc.Terminal,
				Args:     oldProc.Args,
				Env:      oldProc.Env,
				Cwd:      oldProc.Cwd,
				User: &types.User{
					Uid:            oldProc.User.UID,
					Gid:            oldProc.User.GID,
					AdditionalGids: oldProc.User.AdditionalGids,
				},
			})
		}
		state.Containers = append(state.Containers, &types.Container{
			Id:         c.ID(),
			BundlePath: c.Path(),
			Processes:  procs,
			Status:     string(c.State()),
		})
	}
	return state, nil
}

func (s *apiServer) UpdateContainer(ctx context.Context, r *types.UpdateContainerRequest) (*types.UpdateContainerResponse, error) {
	e := supervisor.NewEvent(supervisor.UpdateContainerEventType)
	e.ID = r.Id
	if r.Signal != 0 {
		e.Signal = syscall.Signal(r.Signal)
	}
	e.State = runtime.State(r.Status)
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		return nil, err
	}
	return &types.UpdateContainerResponse{}, nil
}

func (s *apiServer) Events(r *types.EventsRequest, stream types.API_EventsServer) error {
	events := s.sv.Events()
	defer s.sv.Unsubscribe(events)
	for evt := range events {
		var ev *types.Event
		switch evt.Type {
		case supervisor.ExitEventType, supervisor.ExecExitEventType:
			ev = &types.Event{
				Type:   "exit",
				Id:     evt.ID,
				Pid:    evt.Pid,
				Status: uint32(evt.Status),
			}
		case supervisor.OOMEventType:
			ev = &types.Event{
				Type: "oom",
				Id:   evt.ID,
			}
		}
		if ev != nil {
			if err := stream.Send(ev); err != nil {
				return err
			}
		}

	}
	return nil
}

func (s *apiServer) GetStats(r *types.StatsRequest, stream types.API_GetStatsServer) error {
	e := supervisor.NewEvent(supervisor.StatsEventType)
	e.ID = r.Id
	s.sv.SendEvent(e)
	if err := <-e.Err; err != nil {
		if err == supervisor.ErrContainerNotFound {
			return grpc.Errorf(codes.NotFound, err.Error())
		}
		return err
	}
	defer func() {
		ue := supervisor.NewEvent(supervisor.UnsubscribeStatsEventType)
		ue.ID = e.ID
		ue.Stats = e.Stats
		s.sv.SendEvent(ue)
		if err := <-ue.Err; err != nil {
			logrus.Errorf("Error unsubscribing %s: %v", r.Id, err)
		}
	}()
	for {
		select {
		case st := <-e.Stats:
			pbSt, ok := st.(*types.Stats)
			if !ok {
				panic("invalid stats type from collector")
			}
			if err := stream.Send(pbSt); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
	return nil
}
