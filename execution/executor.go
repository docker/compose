package execution

import (
	"io"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type CreateOpts struct {
	Bundle string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type CreateProcessOpts struct {
	Spec   specs.Process
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Executor interface {
	Create(id string, o CreateOpts) (*Container, error)
	Pause(*Container) error
	Resume(*Container) error
	Status(*Container) (Status, error)
	List() ([]*Container, error)
	Load(id string) (*Container, error)
	Delete(*Container) error

	StartProcess(*Container, CreateProcessOpts) (Process, error)
	SignalProcess(*Container, os.Signal) error
	DeleteProcess(*Container, string) error
}
