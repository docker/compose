package execution

import (
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type CreateOpts struct {
	Bundle string
	Stdin  string
	Stdout string
	Stderr string
}

type CreateProcessOpts struct {
	Spec   specs.Process
	Stdin  string
	Stdout string
	Stderr string
}

type Executor interface {
	Create(id string, o CreateOpts) (*Container, error)
	Pause(*Container) error
	Resume(*Container) error
	Status(*Container) (Status, error)
	List() ([]*Container, error)
	Load(id string) (*Container, error)
	Delete(*Container) error
	Start(*Container) error

	StartProcess(*Container, CreateProcessOpts) (Process, error)
	SignalProcess(*Container, string, os.Signal) error
	DeleteProcess(*Container, string) error
}
