package execution

import (
	"context"

	api "github.com/docker/containerd/api/execution"
	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/executors"
)

type Opts struct {
	Root    string
	Runtime string
}

func New(o Opts) (*Service, error) {
	executor := executors.Get(o.Runtime)(o.Root)
	return &Service{
		o:        o,
		executor: executor,
	}, nil
}

type Service struct {
	o        Opts
	executor execution.Executor
}

func (s *Service) Create(ctx context.Context, r *api.CreateContainerRequest) (*api.CreateContainerResponse, error) {
	// TODO: write io and bundle path to dir
	container, err := s.executor.Create(r.ID, r.BundlePath, &IO{})
	if err != nil {
		return nil, err
	}

	s.supervisor.Add(container.InitProcess())

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
func (s *Service) Get(ctx context.Context, r *api.GetContainerRequest) (*api.GetContainerResponse, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	return &api.GetContainerResponse{
		Container: toGRPCContainer(container),
	}, nil
}

func (s *Service) Update(ctx context.Context, r *api.UpdateContainerRequest) (*api.Empty, error) {
	return nil, nil
}

func (s *Service) Pause(ctx context.Context, r *api.PauseContainerRequest) (*api.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	return nil, container.Pause()
}

func (s *Service) Resume(ctx context.Context, r *api.ResumeContainerRequest) (*api.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	return nil, container.Resume()
}

func (s *Service) CreateProcess(ctx context.Context, r *api.CreateProcessRequest) (*api.CreateProcessResponse, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}

	process, err := container.CreateProcess(r.Process)
	if err != nil {
		return nil, err
	}

	s.supervisor.Add(process)

	r.Process.Pid = process.Pid()
	return &api.CreateProcessResponse{
		Process: r.Process,
	}, nil
}

// containerd managed execs + system pids forked in container
func (s *Service) GetProcess(ctx context.Context, r *api.GetProcessRequest) (*api.GetProcessResponse, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	process, err := container.Process(r.ProcessId)
	if err != nil {
		return nil, err
	}
	return &api.GetProcessResponse{
		Process: process,
	}, nil
}

func (s *Service) StartProcess(ctx context.Context, r *api.StartProcessRequest) (*api.StartProcessResponse, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	process, err := container.Process(r.Process.ID)
	if err != nil {
		return nil, err
	}
	if err := process.Start(); err != nil {
		return nil, err
	}
	return &api.StartProcessRequest{
		Process: process,
	}, nil
}

func (s *Service) SignalProcess(ctx context.Context, r *api.SignalProcessRequest) (*api.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	process, err := container.Process(r.Process.ID)
	if err != nil {
		return nil, err
	}
	return nil, process.Signal(r.Signal)
}

func (s *Service) DeleteProcess(ctx context.Context, r *api.DeleteProcessRequest) (*api.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	if err := container.DeleteProcess(r.Process.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) ListProcesses(ctx context.Context, r *api.ListProcessesRequest) (*api.ListProcessesResponse, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	processes, err := container.Processes()
	if err != nil {
		return nil, err
	}
	return &api.ListProcessesResponse{
		Processes: processes,
	}, nil
}

var (
	_ = (api.ExecutionServiceServer)(&Service{})
	_ = (api.ContainerServiceServer)(&Service{})
)
