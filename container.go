package containerd

import (
	"os"

	"github.com/opencontainers/specs"
)

type Process interface {
	Pid() (int, error)
	Spec() specs.Process
	Signal(os.Signal) error
}

type Container interface {
	ID() string
	Start() error
	Path() string
	Pid() (int, error)
	SetExited(status int)
	Delete() error
	Processes() ([]Process, error)
}
