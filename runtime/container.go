package runtime

import (
	"os"
	"time"

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

type Stdio struct {
	Stderr string `json:"stderr,omitempty"`
	Stdout string `json:"stdout,omitempty"`
}

type Checkpoint struct {
	Timestamp   time.Time `json:"timestamp,omitempty"`
	Path        string    `json:"path,omitempty"`
	Name        string    `json:"name,omitempty"`
	Tcp         bool      `json:"tcp"`
	UnixSockets bool      `json:"unixSockets"`
	Shell       bool      `json:"shell"`
	Running     bool      `json:"running,omitempty"`
}

type Container interface {
	// ID returns the container ID
	ID() string
	// Start starts the init process of the container
	Start() error
	// Path returns the path to the bundle
	Path() string
	// Pid returns the container's init process id
	Pid() (int, error)
	// SetExited sets the exit status of the container after it's init dies
	SetExited(status int)
	// Delete deletes the container
	Delete() error
	// Processes returns all the containers processes that have been added
	Processes() ([]Process, error)
	// RemoveProcess removes a specific process for the container because it exited
	RemoveProcess(pid int) error
	// State returns the containers runtime state
	State() State
	// Resume resumes a paused container
	Resume() error
	// Pause pauses a running container
	Pause() error

	Checkpoints() ([]Checkpoint, error)

	Checkpoint(Checkpoint) error

	Restore(path, name string) error
}
