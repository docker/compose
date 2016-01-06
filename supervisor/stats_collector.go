package supervisor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/runtime"
	"github.com/docker/docker/pkg/pubsub"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/system"
)

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

func convertToPb(st *runtime.Stat) *types.Stats {
	pbSt := &types.Stats{
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

type statsPair struct {
	ct  runtime.Container
	pub *pubsub.Publisher
}

func newStatsCollector(interval time.Duration) *statsCollector {
	s := &statsCollector{
		interval:            interval,
		clockTicksPerSecond: uint64(system.GetClockTicks()),
		bufReader:           bufio.NewReaderSize(nil, 128),
		publishers:          make(map[string]*statsPair),
	}
	go s.run()
	return s
}

// statsCollector manages and provides container resource stats
type statsCollector struct {
	m                   sync.Mutex
	supervisor          *Supervisor
	interval            time.Duration
	clockTicksPerSecond uint64
	publishers          map[string]*statsPair
	bufReader           *bufio.Reader
}

// collect registers the container with the collector and adds it to
// the event loop for collection on the specified interval returning
// a channel for the subscriber to receive on.
func (s *statsCollector) collect(c runtime.Container) chan interface{} {
	s.m.Lock()
	defer s.m.Unlock()
	publisher, exists := s.publishers[c.ID()]
	if !exists {
		pub := pubsub.NewPublisher(100*time.Millisecond, 1024)
		publisher = &statsPair{ct: c, pub: pub}
		s.publishers[c.ID()] = publisher
	}
	return publisher.pub.Subscribe()
}

// stopCollection closes the channels for all subscribers and removes
// the container from metrics collection.
func (s *statsCollector) stopCollection(c runtime.Container) {
	s.m.Lock()
	if publisher, exists := s.publishers[c.ID()]; exists {
		publisher.pub.Close()
		delete(s.publishers, c.ID())
	}
	s.m.Unlock()
}

// unsubscribe removes a specific subscriber from receiving updates for a container's stats.
func (s *statsCollector) unsubscribe(c runtime.Container, ch chan interface{}) {
	s.m.Lock()
	publisher := s.publishers[c.ID()]
	if publisher != nil {
		publisher.pub.Evict(ch)
		if publisher.pub.Len() == 0 {
			delete(s.publishers, c.ID())
		}
	}
	s.m.Unlock()
}

func (s *statsCollector) run() {
	type publishersPair struct {
		container runtime.Container
		publisher *pubsub.Publisher
	}
	// we cannot determine the capacity here.
	// it will grow enough in first iteration
	var pairs []*statsPair

	for range time.Tick(s.interval) {
		// it does not make sense in the first iteration,
		// but saves allocations in further iterations
		pairs = pairs[:0]

		s.m.Lock()
		for _, publisher := range s.publishers {
			// copy pointers here to release the lock ASAP
			pairs = append(pairs, publisher)
		}
		s.m.Unlock()
		if len(pairs) == 0 {
			continue
		}

		/*
			for _, pair := range pairs {
					stats, err := pair.ct.Stats()
					if err != nil {
						logrus.Errorf("Error getting stats for container ID %s", pair.ct.ID())
						continue
					}

					pair.pub.Publish(convertToPb(stats))
			}
		*/
	}
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
func (s *statsCollector) getSystemCPUUsage() (uint64, error) {
	var line string
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer func() {
		s.bufReader.Reset(nil)
		f.Close()
	}()
	s.bufReader.Reset(f)
	err = nil
	for err == nil {
		line, err = s.bufReader.ReadString('\n')
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
				s.clockTicksPerSecond, nil
		}
	}
	return 0, fmt.Errorf("bad stats format")
}
