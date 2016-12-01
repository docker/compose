package oci

import (
	"os"
	"syscall"
)

func newProcess(pid int) (*process, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return &process{
		proc: proc,
	}, nil
}

type process struct {
	proc *os.Process
}

func (p *process) Pid() int {
	return p.proc.Pid
}

func (p *process) Wait() (uint32, error) {
	state, err := p.proc.Wait()
	if err != nil {
		return 0, nil
	}
	return uint32(state.Sys().(syscall.WaitStatus).ExitStatus()), nil
}

func (p *process) Signal(s os.Signal) error {
	return p.proc.Signal(s)
}
