package runtime

import (
	"errors"

	"github.com/opencontainers/specs"
)

var (
	ErrNotChildProcess      = errors.New("containerd: not a child process for container")
	ErrInvalidContainerType = errors.New("containerd: invalid container type for runtime")
	ErrCheckpointNotExists  = errors.New("containerd: checkpoint does not exist for container")
)

// runtime handles containers, containers handle their own actions.
type Runtime interface {
	Create(id, bundlePath string, stdio *Stdio) (Container, error)
	StartProcess(Container, specs.Process, *Stdio) (Process, error)
}
