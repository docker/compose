package containerd

import "github.com/opencontainers/specs"

// runtime handles containers, containers handle their own actions.
type Runtime interface {
	Create(id, bundlePath string, stdio *Stdio) (Container, error)
	StartProcess(Container, specs.Process, *Stdio) (Process, error)
}
