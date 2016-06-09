package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docker/containerd/specs"
	ocs "github.com/opencontainers/runtime-spec/specs-go"
)

func findCgroupMountpointAndRoot(pid int, subsystem string) (string, string, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/mountinfo", pid))
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		for _, opt := range strings.Split(fields[len(fields)-1], ",") {
			if opt == subsystem {
				return fields[4], fields[3], nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	return "", "", fmt.Errorf("cgroup path for %s not found", subsystem)
}

func parseCgroupFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	cgroups := make(map[string]string)

	for s.Scan() {
		if err := s.Err(); err != nil {
			return nil, err
		}

		text := s.Text()
		parts := strings.Split(text, ":")

		for _, subs := range strings.Split(parts[1], ",") {
			cgroups[subs] = parts[2]
		}
	}
	return cgroups, nil
}

func (c *container) OOM() (OOM, error) {
	p := c.processes[InitProcessID]
	if p == nil {
		return nil, fmt.Errorf("no init process found")
	}

	mountpoint, hostRoot, err := findCgroupMountpointAndRoot(os.Getpid(), "memory")
	if err != nil {
		return nil, err
	}

	cgroups, err := parseCgroupFile(fmt.Sprintf("/proc/%d/cgroup", p.pid))
	if err != nil {
		return nil, err
	}

	root, ok := cgroups["memory"]
	if !ok {
		return nil, fmt.Errorf("no memory cgroup for container %s", c.ID())
	}

	// Take care of the case were we're running inside a container
	// ourself
	root = strings.TrimPrefix(root, hostRoot)

	return c.getMemeoryEventFD(filepath.Join(mountpoint, root))
}

func u64Ptr(i uint64) *uint64 { return &i }

func (c *container) UpdateResources(r *Resource) error {
	sr := ocs.Resources{
		Memory: &ocs.Memory{
			Limit:       u64Ptr(uint64(r.Memory)),
			Reservation: u64Ptr(uint64(r.MemoryReservation)),
			Swap:        u64Ptr(uint64(r.MemorySwap)),
			Kernel:      u64Ptr(uint64(r.KernelMemory)),
			KernelTCP:   u64Ptr(uint64(r.KernelTCPMemory)),
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
