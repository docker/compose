package containerd

import (
	"errors"
	"io"
	"os"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var ErrNotExecProcess = errors.New("process not an exec process")

type ProcessDelegate interface {
	Pid() int
	Wait() (uint32, error)
	Signal(os.Signal) error
}

type Process struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	exec bool
	s    *specs.Process

	driver Runtime
	c      *Container
	d      ProcessDelegate
}

func (p *Process) Spec() *specs.Process {
	return p.s
}

func (p *Process) Start() error {
	if !p.exec {
		return ErrNotExecProcess
	}
	d, err := p.driver.Exec(p.c, p)
	if err != nil {
		return err
	}
	p.d = d
	return nil
}

func (p *Process) Pid() int {
	return p.d.Pid()
}

func (p *Process) Wait() (uint32, error) {
	return p.d.Wait()
}

func (p *Process) Signal(s os.Signal) error {
	return p.d.Signal(s)
}
