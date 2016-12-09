package execution

import (
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type CreateOpts struct {
	Bundle  string
	Console bool
	Stdin   string
	Stdout  string
	Stderr  string
}

type StartProcessOpts struct {
	Spec    specs.Process
	Console bool
	Stdin   string
	Stdout  string
	Stderr  string
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

	StartProcess(*Container, StartProcessOpts) (Process, error)
	SignalProcess(*Container, string, os.Signal) error
	DeleteProcess(*Container, string) error
}
