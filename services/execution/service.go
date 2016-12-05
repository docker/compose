package execution

import (
	"fmt"

	api "github.com/docker/containerd/api/execution"
	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/execution/executors"
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
)

type Opts struct {
	Root    string
	Runtime string
}

func New(o Opts) (*Service, error) {
	executor := executors.Get(o.Runtime)()
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
	// TODO: open IOs
	container, err := s.executor.Create(r.ID, execution.CreateOpts{
		Bundle: r.BundlePath,
		// Stdin:  r.Stdin,
		// Stdout: r.Stdout,
		// Stderr: r.Stderr,
	})
	if err != nil {
		return nil, err
	}

	s.supervisor.Add(container)

	return &api.CreateContainerResponse{
		Container: toGRPCContainer(container),
	}, nil
}

func (s *Service) Delete(ctx context.Context, r *api.DeleteContainerRequest) (*google_protobuf.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}

	if err = s.executor.Delete(container); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) List(ctx context.Context, r *api.ListContainersRequest) (*api.ListContainersResponse, error) {
	containers, err := s.executor.List()
	if err != nil {
		return nil, err
	}
	resp := &api.ListContainersResponse{}
	for _, c := range containers {
		resp.Containers = append(resp.Containers, toGRPCContainer(c))
	}
	return resp, nil
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

func (s *Service) Update(ctx context.Context, r *api.UpdateContainerRequest) (*google_protobuf.Empty, error) {
	return nil, nil
}

func (s *Service) Pause(ctx context.Context, r *api.PauseContainerRequest) (*google_protobuf.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	return nil, s.executor.Pause(container)
}

func (s *Service) Resume(ctx context.Context, r *api.ResumeContainerRequest) (*google_protobuf.Empty, error) {
	container, err := s.executor.Load(r.ID)
	if err != nil {
		return nil, err
	}
	return nil, s.executor.Resume(container)
}

func (s *Service) StartProcess(ctx context.Context, r *api.StartProcessRequest) (*api.StartProcessResponse, error) {
	container, err := s.executor.Load(r.ContainerId)
	if err != nil {
		return nil, err
	}

	// TODO: generate spec
	var spec specs.Process
	// TODO: open IOs
	process, err := s.executor.StartProcess(container, execution.CreateProcessOpts{
		Spec: spec,
		// Stdin:  r.Stdin,
		// Stdout: r.Stdout,
		// Stderr: r.Stderr,
	})
	if err != nil {
		return nil, err
	}

	s.supervisor.Add(process)

	return &api.StartProcessResponse{
		Process: toGRPCProcess(process),
	}, nil
}

// containerd managed execs + system pids forked in container
func (s *Service) GetProcess(ctx context.Context, r *api.GetProcessRequest) (*api.GetProcessResponse, error) {
	container, err := s.executor.Load(r.Container.ID)
	if err != nil {
		return nil, err
	}
	process := s.executor.GetProcess(r.Pid)
	if process == nil {
		return nil, fmt.Errorf("Make me a constant! Process not foumd!")
	}
	return &api.GetProcessResponse{
		Process: toGRPCProcess(process),
	}, nil
}

func (s *Service) SignalProcess(ctx context.Context, r *api.SignalProcessRequest) (*google_protobuf.Empty, error) {
	container, err := s.executor.Load(r.Container.ID)
	if err != nil {
		return nil, err
	}
	return nil, s.executor.SignalProcess(container, r.Process.ID, r.Signal)
}

func (s *Service) DeleteProcess(ctx context.Context, r *api.DeleteProcessRequest) (*google_protobuf.Empty, error) {
	container, err := s.executor.Load(r.Container.ID)
	if err != nil {
		return nil, err
	}
	if err := s.executor.DeleteProcess(container, r.Process.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Service) ListProcesses(ctx context.Context, r *api.ListProcessesRequest) (*api.ListProcessesResponse, error) {
	container, err := s.executor.Load(r.Container.ID)
	if err != nil {
		return nil, err
	}
	processes := container.Processes()
	return &api.ListProcessesResponse{
		Processes: toGRPCProcesses(processes),
	}, nil
}

var (
	_ = (api.ExecutionServiceServer)(&Service{})
	_ = (api.ContainerServiceServer)(&Service{})
)
