package oci

import (
	"fmt"
	"os"
	"syscall"

	"github.com/docker/containerd/execution"
)

func newProcess(c *execution.Container, id string, pid int) (execution.Process, error) {
	status := execution.Running
	if err := syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			status = execution.Stopped
		} else {
			return nil, err
		}
	}
	return &process{
		c:      c,
		id:     id,
		pid:    pid,
		status: status,
	}, nil
}

type process struct {
	c      *execution.Container
	id     string
	pid    int
	status execution.Status
}

func (p *process) Container() *execution.Container {
	return p.c
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Pid() int64 {
	return int64(p.pid)
}

func (p *process) Wait() (uint32, error) {
	if p.status == execution.Running {
		var wstatus syscall.WaitStatus
		_, err := syscall.Wait4(p.pid, &wstatus, 0, nil)
		if err != nil {
			return 255, nil
		}
		// TODO: implement kill-all if we are the init pid
		p.status = execution.Stopped
		return uint32(wstatus.ExitStatus()), nil
	}

	return 255, execution.ErrProcessNotFound
}

func (p *process) Signal(s os.Signal) error {
	if p.status == execution.Running {
		sig, ok := s.(syscall.Signal)
		if !ok {
			return fmt.Errorf("invalid signal %v", s)
		}
		return syscall.Kill(p.pid, sig)
	}
	return execution.ErrProcessNotFound
}

func (p *process) Status() execution.Status {
	return p.status
}
