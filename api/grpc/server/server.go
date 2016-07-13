package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/supervisor"
	"github.com/golang/protobuf/ptypes"
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

func (s *apiServer) GetServerVersion(ctx context.Context, c *types.GetServerVersionRequest) (*types.GetServerVersionResponse, error) {
	return &types.GetServerVersionResponse{
		Major:    containerd.VersionMajor,
		Minor:    containerd.VersionMinor,
		Patch:    containerd.VersionPatch,
		Revision: containerd.GitCommit,
	}, nil
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
	e.NoPivotRoot = c.NoPivotRoot
	e.Runtime = c.Runtime
	e.RuntimeArgs = c.RuntimeArgs
	e.StartResponse = make(chan supervisor.StartResponse, 1)
	if c.Checkpoint != "" {
		e.CheckpointDir = c.CheckpointDir
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

func (s *apiServer) CreateCheckpoint(ctx context.Context, r *types.CreateCheckpointRequest) (*types.CreateCheckpointResponse, error) {
	e := &supervisor.CreateCheckpointTask{}
	e.ID = r.Id
	e.CheckpointDir = r.CheckpointDir
	e.Checkpoint = &runtime.Checkpoint{
		Name:        r.Checkpoint.Name,
		Exit:        r.Checkpoint.Exit,
		TCP:         r.Checkpoint.Tcp,
		UnixSockets: r.Checkpoint.UnixSockets,
		Shell:       r.Checkpoint.Shell,
		EmptyNS:     r.Checkpoint.EmptyNS,
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
	e.CheckpointDir = r.CheckpointDir
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
	checkpoints, err := container.Checkpoints(r.CheckpointDir)
	if err != nil {
		return nil, err
	}
	for _, c := range checkpoints {
		out = append(out, &types.Checkpoint{
			Name:        c.Name,
			Tcp:         c.TCP,
			Shell:       c.Shell,
			UnixSockets: c.UnixSockets,
			// TODO: figure out timestamp
			//Timestamp:   c.Timestamp,
		})
	}
	return &types.ListCheckpointResponse{Checkpoints: out}, nil
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

func (s *apiServer) State(ctx context.Context, r *types.StateRequest) (*types.StateResponse, error) {

	getState := func(c runtime.Container) (interface{}, error) {
		return createAPIContainer(c, true)
	}

	e := &supervisor.GetContainersTask{}
	e.ID = r.Id
	e.GetState = getState
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
	for idx := range e.Containers {
		state.Containers = append(state.Containers, e.States[idx].(*types.Container))
	}
	return state, nil
}

func createAPIContainer(c runtime.Container, getPids bool) (*types.Container, error) {
	processes, err := c.Processes()
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "get processes for container: "+err.Error())
	}
	var procs []*types.Process
	for _, p := range processes {
		oldProc := p.Spec()
		stdio := p.Stdio()
		proc := &types.Process{
			Pid:       p.ID(),
			SystemPid: uint32(p.SystemPid()),
			Terminal:  oldProc.Terminal,
			Args:      oldProc.Args,
			Env:       oldProc.Env,
			Cwd:       oldProc.Cwd,
			Stdin:     stdio.Stdin,
			Stdout:    stdio.Stdout,
			Stderr:    stdio.Stderr,
		}
		proc.User = &types.User{
			Uid:            oldProc.User.UID,
			Gid:            oldProc.User.GID,
			AdditionalGids: oldProc.User.AdditionalGids,
		}
		proc.Capabilities = oldProc.Capabilities
		proc.ApparmorProfile = oldProc.ApparmorProfile
		proc.SelinuxLabel = oldProc.SelinuxLabel
		proc.NoNewPrivileges = oldProc.NoNewPrivileges
		for _, rl := range oldProc.Rlimits {
			proc.Rlimits = append(proc.Rlimits, &types.Rlimit{
				Type: rl.Type,
				Soft: rl.Soft,
				Hard: rl.Hard,
			})
		}
		procs = append(procs, proc)
	}
	var pids []int
	state := c.State()
	if getPids && (state == runtime.Running || state == runtime.Paused) {
		if pids, err = c.Pids(); err != nil {
			return nil, grpc.Errorf(codes.Internal, "get all pids for container: "+err.Error())
		}
	}
	return &types.Container{
		Id:         c.ID(),
		BundlePath: c.Path(),
		Processes:  procs,
		Labels:     c.Labels(),
		Status:     string(state),
		Pids:       toUint32(pids),
		Runtime:    c.Runtime(),
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
	if r.Resources != nil {
		rs := r.Resources
		e.Resources = &runtime.Resource{}
		if rs.CpuShares != 0 {
			e.Resources.CPUShares = int64(rs.CpuShares)
		}
		if rs.BlkioWeight != 0 {
			e.Resources.BlkioWeight = uint16(rs.BlkioWeight)
		}
		if rs.CpuPeriod != 0 {
			e.Resources.CPUPeriod = int64(rs.CpuPeriod)
		}
		if rs.CpuQuota != 0 {
			e.Resources.CPUQuota = int64(rs.CpuQuota)
		}
		if rs.CpusetCpus != "" {
			e.Resources.CpusetCpus = rs.CpusetCpus
		}
		if rs.CpusetMems != "" {
			e.Resources.CpusetMems = rs.CpusetMems
		}
		if rs.KernelMemoryLimit != 0 {
			e.Resources.KernelMemory = int64(rs.KernelMemoryLimit)
		}
		if rs.KernelTCPMemoryLimit != 0 {
			e.Resources.KernelTCPMemory = int64(rs.KernelTCPMemoryLimit)
		}
		if rs.MemoryLimit != 0 {
			e.Resources.Memory = int64(rs.MemoryLimit)
		}
		if rs.MemoryReservation != 0 {
			e.Resources.MemoryReservation = int64(rs.MemoryReservation)
		}
		if rs.MemorySwap != 0 {
			e.Resources.MemorySwap = int64(rs.MemorySwap)
		}
	}
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
	if r.Timestamp != nil {
		from, err := ptypes.Timestamp(r.Timestamp)
		if err != nil {
			return err
		}
		t = from
	}
	if r.StoredOnly && t.IsZero() {
		return fmt.Errorf("invalid parameter: StoredOnly cannot be specified without setting a valid Timestamp")
	}
	events := s.sv.Events(t, r.StoredOnly, r.Id)
	defer s.sv.Unsubscribe(events)
	for e := range events {
		tsp, err := ptypes.TimestampProto(e.Timestamp)
		if err != nil {
			return err
		}
		if r.Id == "" || e.ID == r.Id {
			if err := stream.Send(&types.Event{
				Id:        e.ID,
				Type:      e.Type,
				Timestamp: tsp,
				Pid:       e.PID,
				Status:    uint32(e.Status),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func convertToPb(st *runtime.Stat) *types.StatsResponse {
	tsp, _ := ptypes.TimestampProto(st.Timestamp)
	pbSt := &types.StatsResponse{
		Timestamp:   tsp,
		CgroupStats: &types.CgroupStats{},
	}
	systemUsage, _ := getSystemCPUUsage()
	pbSt.CgroupStats.CpuStats = &types.CpuStats{
		CpuUsage: &types.CpuUsage{
			TotalUsage:        st.CPU.Usage.Total,
			PercpuUsage:       st.CPU.Usage.Percpu,
			UsageInKernelmode: st.CPU.Usage.Kernel,
			UsageInUsermode:   st.CPU.Usage.User,
		},
		ThrottlingData: &types.ThrottlingData{
			Periods:          st.CPU.Throttling.Periods,
			ThrottledPeriods: st.CPU.Throttling.ThrottledPeriods,
			ThrottledTime:    st.CPU.Throttling.ThrottledTime,
		},
		SystemUsage: systemUsage,
	}
	pbSt.CgroupStats.MemoryStats = &types.MemoryStats{
		Cache: st.Memory.Cache,
		Usage: &types.MemoryData{
			Usage:    st.Memory.Usage.Usage,
			MaxUsage: st.Memory.Usage.Max,
			Failcnt:  st.Memory.Usage.Failcnt,
			Limit:    st.Memory.Usage.Limit,
		},
		SwapUsage: &types.MemoryData{
			Usage:    st.Memory.Swap.Usage,
			MaxUsage: st.Memory.Swap.Max,
			Failcnt:  st.Memory.Swap.Failcnt,
			Limit:    st.Memory.Swap.Limit,
		},
		KernelUsage: &types.MemoryData{
			Usage:    st.Memory.Kernel.Usage,
			MaxUsage: st.Memory.Kernel.Max,
			Failcnt:  st.Memory.Kernel.Failcnt,
			Limit:    st.Memory.Kernel.Limit,
		},
		Stats: st.Memory.Raw,
	}
	pbSt.CgroupStats.BlkioStats = &types.BlkioStats{
		IoServiceBytesRecursive: convertBlkioEntryToPb(st.Blkio.IoServiceBytesRecursive),
		IoServicedRecursive:     convertBlkioEntryToPb(st.Blkio.IoServicedRecursive),
		IoQueuedRecursive:       convertBlkioEntryToPb(st.Blkio.IoQueuedRecursive),
		IoServiceTimeRecursive:  convertBlkioEntryToPb(st.Blkio.IoServiceTimeRecursive),
		IoWaitTimeRecursive:     convertBlkioEntryToPb(st.Blkio.IoWaitTimeRecursive),
		IoMergedRecursive:       convertBlkioEntryToPb(st.Blkio.IoMergedRecursive),
		IoTimeRecursive:         convertBlkioEntryToPb(st.Blkio.IoTimeRecursive),
		SectorsRecursive:        convertBlkioEntryToPb(st.Blkio.SectorsRecursive),
	}
	pbSt.CgroupStats.HugetlbStats = make(map[string]*types.HugetlbStats)
	for k, st := range st.Hugetlb {
		pbSt.CgroupStats.HugetlbStats[k] = &types.HugetlbStats{
			Usage:    st.Usage,
			MaxUsage: st.Max,
			Failcnt:  st.Failcnt,
		}
	}
	pbSt.CgroupStats.PidsStats = &types.PidsStats{
		Current: st.Pids.Current,
		Limit:   st.Pids.Limit,
	}
	return pbSt
}

func convertBlkioEntryToPb(b []runtime.BlkioEntry) []*types.BlkioStatsEntry {
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

const nanoSecondsPerSecond = 1e9

// getSystemCPUUsage returns the host system's cpu usage in
// nanoseconds. An error is returned if the format of the underlying
// file does not match.
//
// Uses /proc/stat defined by POSIX. Looks for the cpu
// statistics line and then sums up the first seven fields
// provided. See `man 5 proc` for details on specific field
// information.
func getSystemCPUUsage() (uint64, error) {
	var line string
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	bufReader := bufio.NewReaderSize(nil, 128)
	defer func() {
		bufReader.Reset(nil)
		f.Close()
	}()
	bufReader.Reset(f)
	err = nil
	for err == nil {
		line, err = bufReader.ReadString('\n')
		if err != nil {
			break
		}
		parts := strings.Fields(line)
		switch parts[0] {
		case "cpu":
			if len(parts) < 8 {
				return 0, fmt.Errorf("bad format of cpu stats")
			}
			var totalClockTicks uint64
			for _, i := range parts[1:8] {
				v, err := strconv.ParseUint(i, 10, 64)
				if err != nil {
					return 0, fmt.Errorf("error parsing cpu stats")
				}
				totalClockTicks += v
			}
			return (totalClockTicks * nanoSecondsPerSecond) /
				clockTicksPerSecond, nil
		}
	}
	return 0, fmt.Errorf("bad stats format")
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
