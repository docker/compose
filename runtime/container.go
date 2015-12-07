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
	Status Status
}

type Stdio struct {
	Stderr string
	Stdout string
}

type Checkpoint struct {
	// Timestamp is the time that checkpoint happened
	Timestamp time.Time
	// Name is the name of the checkpoint
	Name string
	// Tcp checkpoints open tcp connections
	Tcp bool
	// UnixSockets persists unix sockets in the checkpoint
	UnixSockets bool
	// Shell persists tty sessions in the checkpoint
	Shell bool
	// Exit exits the container after the checkpoint is finished
	Exit bool
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
	// Checkpoints returns all the checkpoints for a container
	Checkpoints() ([]Checkpoint, error)
	// Checkpoint creates a new checkpoint
	Checkpoint(Checkpoint) error
	// DeleteCheckpoint deletes the checkpoint for the provided name
	DeleteCheckpoint(name string) error
	// Restore restores the container to that of the checkpoint provided by name
	Restore(name string) error
}
