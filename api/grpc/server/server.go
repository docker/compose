package server

import (
	"errors"
	"fmt"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/supervisor"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
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
	e := &supervisor.StartTask{}
	e.ID = c.Id
	e.BundlePath = c.BundlePath
	e.Stdin = c.Stdin
	e.Stdout = c.Stdout
	e.Stderr = c.Stderr
	e.Labels = c.Labels
	e.StartResponse = make(chan supervisor.StartResponse, 1)
	if c.Checkpoint != "" {
		e.Checkpoint = &runtime.Checkpoint{
			Name: c.Checkpoint,
		}
	}
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	r := <-e.StartResponse
	apiC, err := createAPIContainer(r.Container, false)
	if err != nil {
		return nil, err
	}
	return &types.CreateContainerResponse{
		Container: apiC,
	}, nil
}

func (s *apiServer) Signal(ctx context.Context, r *types.SignalRequest) (*types.SignalResponse, error) {
	e := &supervisor.SignalTask{}
	e.ID = r.Id
	e.PID = r.Pid
	e.Signal = syscall.Signal(int(r.Signal))
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
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
	e := &supervisor.AddProcessTask{}
	e.ID = r.Id
	e.PID = r.Pid
	e.ProcessSpec = process
	e.Stdin = r.Stdin
	e.Stdout = r.Stdout
	e.Stderr = r.Stderr
	e.StartResponse = make(chan supervisor.StartResponse, 1)
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	<-e.StartResponse
	return &types.AddProcessResponse{}, nil
}

func (s *apiServer) CreateCheckpoint(ctx context.Context, r *types.CreateCheckpointRequest) (*types.CreateCheckpointResponse, error) {
	e := &supervisor.CreateCheckpointTask{}
	e.ID = r.Id
	e.Checkpoint = &runtime.Checkpoint{
		Name:        r.Checkpoint.Name,
		Exit:        r.Checkpoint.Exit,
		Tcp:         r.Checkpoint.Tcp,
		UnixSockets: r.Checkpoint.UnixSockets,
		Shell:       r.Checkpoint.Shell,
	}
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	return &types.CreateCheckpointResponse{}, nil
}

func (s *apiServer) DeleteCheckpoint(ctx context.Context, r *types.DeleteCheckpointRequest) (*types.DeleteCheckpointResponse, error) {
	if r.Name == "" {
		return nil, errors.New("checkpoint name cannot be empty")
	}
	e := &supervisor.DeleteCheckpointTask{}
	e.ID = r.Id
	e.Checkpoint = &runtime.Checkpoint{
		Name: r.Name,
	}
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	return &types.DeleteCheckpointResponse{}, nil
}

func (s *apiServer) ListCheckpoint(ctx context.Context, r *types.ListCheckpointRequest) (*types.ListCheckpointResponse, error) {
	e := &supervisor.GetContainersTask{}
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
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
	return &types.ListCheckpointResponse{Checkpoints: out}, nil
}

func (s *apiServer) State(ctx context.Context, r *types.StateRequest) (*types.StateResponse, error) {
	e := &supervisor.GetContainersTask{}
	e.ID = r.Id
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	m := s.sv.Machine()
	state := &types.StateResponse{
		Machine: &types.Machine{
			Cpus:   uint32(m.Cpus),
			Memory: uint64(m.Memory),
		},
	}
	for _, c := range e.Containers {
		apiC, err := createAPIContainer(c, true)
		if err != nil {
			return nil, err
		}
		state.Containers = append(state.Containers, apiC)
	}
	return state, nil
}

func createAPIContainer(c runtime.Container, getPids bool) (*types.Container, error) {
	processes, err := c.Processes()
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "get processes for container")
	}
	var procs []*types.Process
	for _, p := range processes {
		oldProc := p.Spec()
		stdio := p.Stdio()
		procs = append(procs, &types.Process{
			Pid:       p.ID(),
			SystemPid: uint32(p.SystemPid()),
			Terminal:  oldProc.Terminal,
			Args:      oldProc.Args,
			Env:       oldProc.Env,
			Cwd:       oldProc.Cwd,
			Stdin:     stdio.Stdin,
			Stdout:    stdio.Stdout,
			Stderr:    stdio.Stderr,
			User: &types.User{
				Uid:            oldProc.User.UID,
				Gid:            oldProc.User.GID,
				AdditionalGids: oldProc.User.AdditionalGids,
			},
		})
	}
	var pids []int
	if getPids {
		if pids, err = c.Pids(); err != nil {
			return nil, grpc.Errorf(codes.Internal, "get all pids for container")
		}
	}
	return &types.Container{
		Id:         c.ID(),
		BundlePath: c.Path(),
		Processes:  procs,
		Labels:     c.Labels(),
		Status:     string(c.State()),
		Pids:       toUint32(pids),
	}, nil
}

func toUint32(its []int) []uint32 {
	o := []uint32{}
	for _, i := range its {
		o = append(o, uint32(i))
	}
	return o
}

func (s *apiServer) UpdateContainer(ctx context.Context, r *types.UpdateContainerRequest) (*types.UpdateContainerResponse, error) {
	e := &supervisor.UpdateTask{}
	e.ID = r.Id
	e.State = runtime.State(r.Status)
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	return &types.UpdateContainerResponse{}, nil
}

func (s *apiServer) UpdateProcess(ctx context.Context, r *types.UpdateProcessRequest) (*types.UpdateProcessResponse, error) {
	e := &supervisor.UpdateProcessTask{}
	e.ID = r.Id
	e.PID = r.Pid
	e.Height = int(r.Height)
	e.Width = int(r.Width)
	e.CloseStdin = r.CloseStdin
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	return &types.UpdateProcessResponse{}, nil
}

func (s *apiServer) Events(r *types.EventsRequest, stream types.API_EventsServer) error {
	t := time.Time{}
	if r.Timestamp != 0 {
		t = time.Unix(int64(r.Timestamp), 0)
	}
	events := s.sv.Events(t)
	defer s.sv.Unsubscribe(events)
	for e := range events {
		if err := stream.Send(&types.Event{
			Id:        e.ID,
			Type:      e.Type,
			Timestamp: uint64(e.Timestamp.Unix()),
			Pid:       e.PID,
			Status:    uint32(e.Status),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *apiServer) Stats(ctx context.Context, r *types.StatsRequest) (*types.StatsResponse, error) {
	e := &supervisor.StatsTask{}
	e.ID = r.Id
	e.Stat = make(chan *runtime.Stat, 1)
	s.sv.SendTask(e)
	if err := <-e.ErrorCh(); err != nil {
		return nil, err
	}
	stats := <-e.Stat
	t := convertToPb(stats)
	return t, nil
}

func convertToPb(st *runtime.Stat) *types.StatsResponse {
	pbSt := &types.StatsResponse{
		Timestamp:   uint64(st.Timestamp.Unix()),
		CgroupStats: &types.CgroupStats{},
	}
	lcSt, ok := st.Data.(*libcontainer.Stats)
	if !ok {
		return pbSt
	}
	cpuSt := lcSt.CgroupStats.CpuStats
	pbSt.CgroupStats.CpuStats = &types.CpuStats{
		CpuUsage: &types.CpuUsage{
			TotalUsage:        cpuSt.CpuUsage.TotalUsage,
			PercpuUsage:       cpuSt.CpuUsage.PercpuUsage,
			UsageInKernelmode: cpuSt.CpuUsage.UsageInKernelmode,
			UsageInUsermode:   cpuSt.CpuUsage.UsageInUsermode,
		},
		ThrottlingData: &types.ThrottlingData{
			Periods:          cpuSt.ThrottlingData.Periods,
			ThrottledPeriods: cpuSt.ThrottlingData.ThrottledPeriods,
			ThrottledTime:    cpuSt.ThrottlingData.ThrottledTime,
		},
	}
	memSt := lcSt.CgroupStats.MemoryStats
	pbSt.CgroupStats.MemoryStats = &types.MemoryStats{
		Cache: memSt.Cache,
		Usage: &types.MemoryData{
			Usage:    memSt.Usage.Usage,
			MaxUsage: memSt.Usage.MaxUsage,
			Failcnt:  memSt.Usage.Failcnt,
		},
		SwapUsage: &types.MemoryData{
			Usage:    memSt.SwapUsage.Usage,
			MaxUsage: memSt.SwapUsage.MaxUsage,
			Failcnt:  memSt.SwapUsage.Failcnt,
		},
	}
	blkSt := lcSt.CgroupStats.BlkioStats
	pbSt.CgroupStats.BlkioStats = &types.BlkioStats{
		IoServiceBytesRecursive: convertBlkioEntryToPb(blkSt.IoServiceBytesRecursive),
		IoServicedRecursive:     convertBlkioEntryToPb(blkSt.IoServicedRecursive),
		IoQueuedRecursive:       convertBlkioEntryToPb(blkSt.IoQueuedRecursive),
		IoServiceTimeRecursive:  convertBlkioEntryToPb(blkSt.IoServiceTimeRecursive),
		IoWaitTimeRecursive:     convertBlkioEntryToPb(blkSt.IoWaitTimeRecursive),
		IoMergedRecursive:       convertBlkioEntryToPb(blkSt.IoMergedRecursive),
		IoTimeRecursive:         convertBlkioEntryToPb(blkSt.IoTimeRecursive),
		SectorsRecursive:        convertBlkioEntryToPb(blkSt.SectorsRecursive),
	}
	pbSt.CgroupStats.HugetlbStats = make(map[string]*types.HugetlbStats)
	for k, st := range lcSt.CgroupStats.HugetlbStats {
		pbSt.CgroupStats.HugetlbStats[k] = &types.HugetlbStats{
			Usage:    st.Usage,
			MaxUsage: st.MaxUsage,
			Failcnt:  st.Failcnt,
		}
	}
	return pbSt
}

func convertBlkioEntryToPb(b []cgroups.BlkioStatEntry) []*types.BlkioStatsEntry {
	var pbEs []*types.BlkioStatsEntry
	for _, e := range b {
		pbEs = append(pbEs, &types.BlkioStatsEntry{
			Major: e.Major,
			Minor: e.Minor,
			Op:    e.Op,
			Value: e.Value,
		})
	}
	return pbEs
}
