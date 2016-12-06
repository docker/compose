package oci

import (
	"os"
	"syscall"

	"github.com/docker/containerd/execution"
)

func newProcess(id string, pid int) (execution.Process, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return &process{
		id:   id,
		proc: proc,
	}, nil
}

type process struct {
	id   string
	proc *os.Process
}

func (p *process) ID() string {
	return p.id
}

func (p *process) Pid() int64 {
	return int64(p.proc.Pid)
}

func (p *process) Wait() (uint32, error) {
	state, err := p.proc.Wait()
	if err != nil {
		return 0, nil
	}
	// TODO: implement kill-all if we are the init pid
	return uint32(state.Sys().(syscall.WaitStatus).ExitStatus()), nil
}

func (p *process) Signal(s os.Signal) error {
	return p.proc.Signal(s)
}
