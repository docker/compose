package runtime

import (
	"errors"

	"github.com/opencontainers/specs"
)

var (
	ErrNotChildProcess      = errors.New("containerd: not a child process for container")
	ErrInvalidContainerType = errors.New("containerd: invalid container type for runtime")
	ErrCheckpointNotExists  = errors.New("containerd: checkpoint does not exist for container")
	ErrCheckpointExists     = errors.New("containerd: checkpoint already exists")
	ErrContainerExited      = errors.New("containerd: container has exited")
)

// Runtime handles containers, containers handle their own actions
type Runtime interface {
	// Create creates a new container initialized but without it starting it
	Create(id, bundlePath string, stdio *Stdio) (Container, error)
	// StartProcess adds a new process to the container
	StartProcess(Container, specs.Process, *Stdio) (Process, error)
	Type() string
}
