package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docker/containerd/specs"
	"github.com/opencontainers/runc/libcontainer"
	ocs "github.com/opencontainers/runtime-spec/specs-go"
)

func (c *container) getLibctContainer() (libcontainer.Container, error) {
	runtimeRoot := "/run/runc"

	// Check that the root wasn't changed
	for _, opt := range c.runtimeArgs {
		if strings.HasPrefix(opt, "--root=") {
			runtimeRoot = strings.TrimPrefix(opt, "--root=")
			break
		}
	}

	f, err := libcontainer.New(runtimeRoot, libcontainer.Cgroupfs)
	if err != nil {
		return nil, err
	}
	return f.Load(c.id)
}

func (c *container) OOM() (OOM, error) {
	container, err := c.getLibctContainer()
	if err != nil {
		if lerr, ok := err.(libcontainer.Error); ok {
			// with oom registration sometimes the container can run, exit, and be destroyed
			// faster than we can get the state back so we can just ignore this
			if lerr.Code() == libcontainer.ContainerNotExists {
				return nil, ErrContainerExited
			}
		}
		return nil, err
	}
	state, err := container.State()
	if err != nil {
		return nil, err
	}
	memoryPath := state.CgroupPaths["memory"]
	return c.getMemeoryEventFD(memoryPath)
}

func u64Ptr(i uint64) *uint64 { return &i }

func (c *container) UpdateResources(r *Resource) error {
	sr := ocs.Resources{
		Memory: &ocs.Memory{
			Limit:       u64Ptr(uint64(r.Memory)),
			Reservation: u64Ptr(uint64(r.MemoryReservation)),
			Swap:        u64Ptr(uint64(r.MemorySwap)),
			Kernel:      u64Ptr(uint64(r.KernelMemory)),
		},
		CPU: &ocs.CPU{
			Shares: u64Ptr(uint64(r.CPUShares)),
			Quota:  u64Ptr(uint64(r.CPUQuota)),
			Period: u64Ptr(uint64(r.CPUPeriod)),
			Cpus:   &r.CpusetCpus,
			Mems:   &r.CpusetMems,
		},
		BlockIO: &ocs.BlockIO{
			Weight: &r.BlkioWeight,
		},
	}

	srStr := bytes.NewBuffer(nil)
	if err := json.NewEncoder(srStr).Encode(&sr); err != nil {
		return err
	}

	args := c.runtimeArgs
	args = append(args, "update", "-r", "-", c.id)
	cmd := exec.Command(c.runtime, args...)
	cmd.Stdin = srStr
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(b))
	}
	return nil
}

func getRootIDs(s *specs.Spec) (int, int, error) {
	if s == nil {
		return 0, 0, nil
	}
	var hasUserns bool
	for _, ns := range s.Linux.Namespaces {
		if ns.Type == ocs.UserNamespace {
			hasUserns = true
			break
		}
	}
	if !hasUserns {
		return 0, 0, nil
	}
	uid := hostIDFromMap(0, s.Linux.UIDMappings)
	gid := hostIDFromMap(0, s.Linux.GIDMappings)
	return uid, gid, nil
}

func (c *container) getMemeoryEventFD(root string) (*oom, error) {
	f, err := os.Open(filepath.Join(root, "memory.oom_control"))
	if err != nil {
		return nil, err
	}
	fd, _, serr := syscall.RawSyscall(syscall.SYS_EVENTFD2, 0, syscall.FD_CLOEXEC, 0)
	if serr != 0 {
		f.Close()
		return nil, serr
	}
	if err := c.writeEventFD(root, int(f.Fd()), int(fd)); err != nil {
		syscall.Close(int(fd))
		f.Close()
		return nil, err
	}
	return &oom{
		root:    root,
		id:      c.id,
		eventfd: int(fd),
		control: f,
	}, nil
}
