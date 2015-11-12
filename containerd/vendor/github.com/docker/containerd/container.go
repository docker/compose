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
type Status string

const (
	Paused  Status = "paused"
	Running Status = "running"
)

type State struct {
	Status Status `json:"status,omitempty"`
}

type Container interface {
	ID() string
	Start() error
	Path() string
	Pid() (int, error)
	SetExited(status int)
	Delete() error
	Processes() ([]Process, error)
	RemoveProcess(pid int) error
	State() State
	Resume() error
	Pause() error
}
