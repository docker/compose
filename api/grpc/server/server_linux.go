package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/containerd/specs"
	"github.com/docker/containerd/supervisor"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/system"
	ocs "github.com/opencontainers/specs"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func createContainerConfigCheckpoint(e *supervisor.StartTask, c *types.CreateContainerRequest) {
	if c.Checkpoint != "" {
		e.Checkpoint = &runtime.Checkpoint{
			Name: c.Checkpoint,
		}
	}
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
	systemUsage, _ := getSystemCPUUsage()
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
		SystemUsage: systemUsage,
	}
	memSt := lcSt.CgroupStats.MemoryStats
	pbSt.CgroupStats.MemoryStats = &types.MemoryStats{
		Cache: memSt.Cache,
		Usage: &types.MemoryData{
			Usage:    memSt.Usage.Usage,
			MaxUsage: memSt.Usage.MaxUsage,
			Failcnt:  memSt.Usage.Failcnt,
			Limit:    memSt.Usage.Limit,
		},
		SwapUsage: &types.MemoryData{
			Usage:    memSt.SwapUsage.Usage,
			MaxUsage: memSt.SwapUsage.MaxUsage,
			Failcnt:  memSt.SwapUsage.Failcnt,
			Limit:    memSt.SwapUsage.Limit,
		},
		KernelUsage: &types.MemoryData{
			Usage:    memSt.KernelUsage.Usage,
			MaxUsage: memSt.KernelUsage.MaxUsage,
			Failcnt:  memSt.KernelUsage.Failcnt,
			Limit:    memSt.KernelUsage.Limit,
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

const nanoSecondsPerSecond = 1e9

var clockTicksPerSecond = uint64(system.GetClockTicks())

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

func setUserFieldsInProcess(p *types.Process, oldProc specs.ProcessSpec) {
	p.User = &types.User{
		Uid:            oldProc.User.UID,
		Gid:            oldProc.User.GID,
		AdditionalGids: oldProc.User.AdditionalGids,
	}
	p.Capabilities = oldProc.Capabilities
	p.ApparmorProfile = oldProc.ApparmorProfile
	p.SelinuxLabel = oldProc.SelinuxLabel
	p.NoNewPrivileges = oldProc.NoNewPrivileges
}

func setPlatformRuntimeProcessSpecUserFields(r *types.AddProcessRequest, process *specs.ProcessSpec) {
	process.User = ocs.User{
		UID:            r.User.Uid,
		GID:            r.User.Gid,
		AdditionalGids: r.User.AdditionalGids,
	}
	process.Capabilities = r.Capabilities
	process.ApparmorProfile = r.ApparmorProfile
	process.SelinuxLabel = r.SelinuxLabel
	process.NoNewPrivileges = r.NoNewPrivileges
}
