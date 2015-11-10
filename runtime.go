package containerd

import "github.com/opencontainers/specs"

// runtime handles containers, containers handle their own actions.
type Runtime interface {
	Create(id, bundlePath string) (Container, error)
	StartProcess(Container, specs.Process) (Process, error)
}
