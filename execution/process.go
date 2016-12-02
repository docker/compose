package execution

import (
	"io"
	"os"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type ProcessController interface {
	Start(*Process) error
	Status(*Process) (Status, error)
	Wait(*Process) (uint32, error)
	Signal(*Process, os.Signal) error
}

type Process struct {
	Pid    int
	Spec   *specs.Process
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	controller ProcessController
}

func (p *Process) Status() (Status, error) {
	return p.controller.Status(p)
}

func (p *Process) Wait() (uint32, error) {
	return p.controller.Wait(p)
}

func (p *Process) Signal(s os.Signal) error {
	return p.controller.Signal(p, s)
}

func (p *Process) Start() error {
	return p.controller.Start(p)
}
